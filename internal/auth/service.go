package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

var ErrUnauthorized = errors.New("unauthorized")

type Service struct {
	db                    *sql.DB
	secret                []byte
	refreshSecret         []byte
	accessTTL, refreshTTL time.Duration
	mu                    sync.Mutex
	rate                  map[string]window
}
type window struct {
	started time.Time
	count   int
}
type Principal struct {
	UserID, Username, APIKeyID, SessionID string
	IsAPIKey                              bool
}
type APIKey struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	KeyPrefix  string    `json:"key_prefix"`
	SessionID  string    `json:"session_id"`
	AllowedIPs []string  `json:"allowed_ips"`
	RateLimit  *int      `json:"rate_limit"`
	IsActive   bool      `json:"is_active"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func New(db *sql.DB, secret, refreshSecret string) *Service {
	return &Service{db: db, secret: []byte(secret), refreshSecret: []byte(refreshSecret), accessTTL: 15 * time.Minute, refreshTTL: 7 * 24 * time.Hour, rate: map[string]window{}}
}

func (s *Service) EnsureAdmin(ctx context.Context, username, password string) error {
	if username == "" || password == "" {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO users(username,password_hash) VALUES($1,$2) ON CONFLICT(username) DO UPDATE SET password_hash=EXCLUDED.password_hash, updated_at=now()`, username, string(hash))
	return err
}

func (s *Service) Login(ctx context.Context, username, password string) (string, string, int, error) {
	var id, hash string
	if err := s.db.QueryRowContext(ctx, `SELECT id, password_hash FROM users WHERE username=$1`, username).Scan(&id, &hash); err != nil {
		return "", "", 0, ErrUnauthorized
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return "", "", 0, ErrUnauthorized
	}
	return s.issue(ctx, id, username)
}

func (s *Service) issue(ctx context.Context, userID, username string) (string, string, int, error) {
	now := time.Now()
	accessExp := now.Add(s.accessTTL)
	refreshExp := now.Add(s.refreshTTL)
	access := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": userID, "username": username, "exp": accessExp.Unix(), "iat": now.Unix(), "iss": "wa-gateway"})
	accessToken, err := access.SignedString(s.secret)
	if err != nil {
		return "", "", 0, err
	}
	refresh := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": userID, "username": username, "exp": refreshExp.Unix(), "iat": now.Unix(), "iss": "wa-gateway"})
	refreshToken, err := refresh.SignedString(s.refreshSecret)
	if err != nil {
		return "", "", 0, err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE users SET refresh_token_hash=$1, updated_at=now() WHERE id=$2`, hash(refreshToken), userID)
	if err != nil {
		return "", "", 0, err
	}
	return accessToken, refreshToken, int(s.accessTTL.Seconds()), nil
}

func (s *Service) Refresh(ctx context.Context, token string) (string, string, int, error) {
	claims, err := parse(token, s.refreshSecret)
	if err != nil {
		return "", "", 0, ErrUnauthorized
	}
	userID, _ := claims["sub"].(string)
	username, _ := claims["username"].(string)
	var stored string
	if err := s.db.QueryRowContext(ctx, `SELECT refresh_token_hash FROM users WHERE id=$1`, userID).Scan(&stored); err != nil || stored != hash(token) {
		return "", "", 0, ErrUnauthorized
	}
	return s.issue(ctx, userID, username)
}

func (s *Service) VerifyAccess(token string) (Principal, error) {
	claims, err := parse(token, s.secret)
	if err != nil {
		return Principal{}, ErrUnauthorized
	}
	userID, _ := claims["sub"].(string)
	username, _ := claims["username"].(string)
	return Principal{UserID: userID, Username: username}, nil
}

func (s *Service) VerifyAPIKey(ctx context.Context, value, remoteIP string) (Principal, error) {
	if value == "" {
		return Principal{}, ErrUnauthorized
	}
	var p Principal
	var allowed []string
	var rateLimit sql.NullInt64
	p.IsAPIKey = true
	if err := s.db.QueryRowContext(ctx, `SELECT id, COALESCE(session_id,''), allowed_ips, rate_limit FROM api_keys WHERE key_hash=$1 AND is_active=true`, hash(value)).Scan(&p.APIKeyID, &p.SessionID, pq.Array(&allowed), &rateLimit); err != nil {
		return Principal{}, ErrUnauthorized
	}
	if len(allowed) > 0 && !ipAllowed(remoteIP, allowed) {
		return Principal{}, ErrUnauthorized
	}
	if rateLimit.Valid && rateLimit.Int64 > 0 {
		s.mu.Lock()
		now := time.Now()
		w := s.rate[p.APIKeyID]
		if now.Sub(w.started) >= time.Minute {
			w = window{started: now}
		}
		if w.count >= int(rateLimit.Int64) {
			s.mu.Unlock()
			return Principal{}, ErrUnauthorized
		}
		w.count++
		s.rate[p.APIKeyID] = w
		s.mu.Unlock()
	}
	return p, nil
}

func ipAllowed(value string, rules []string) bool {
	ip := net.ParseIP(value)
	if ip == nil {
		return false
	}
	for _, rule := range rules {
		rule = strings.TrimSpace(rule)
		if parsed := net.ParseIP(rule); parsed != nil && parsed.Equal(ip) {
			return true
		}
		if _, network, err := net.ParseCIDR(rule); err == nil && network.Contains(ip) {
			return true
		}
	}
	return false
}

func (s *Service) CreateAPIKey(ctx context.Context, name string, allowedIPs []string, rateLimit *int, sessionID string, active bool) (APIKey, string, error) {
	key := GenerateAPIKey()
	prefix := key[:12]
	id := uuid.NewString()
	var out APIKey
	err := s.db.QueryRowContext(ctx, `INSERT INTO api_keys(id,key_hash,key_prefix,name,allowed_ips,rate_limit,session_id,is_active) VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),$8) RETURNING id,name,key_prefix,COALESCE(session_id,''),COALESCE(allowed_ips, ARRAY[]::text[]),rate_limit,is_active,created_at,updated_at`, id, hash(key), prefix, name, allowedIPs, rateLimit, sessionID, active).Scan(&out.ID, &out.Name, &out.KeyPrefix, &out.SessionID, pq.Array(&out.AllowedIPs), &out.RateLimit, &out.IsActive, &out.CreatedAt, &out.UpdatedAt)
	return out, key, err
}

func (s *Service) ListAPIKeys(ctx context.Context) ([]APIKey, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,key_prefix,COALESCE(session_id,''),COALESCE(allowed_ips, ARRAY[]::text[]),rate_limit,is_active,created_at,updated_at FROM api_keys ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APIKey
	for rows.Next() {
		var v APIKey
		if err := rows.Scan(&v.ID, &v.Name, &v.KeyPrefix, &v.SessionID, pq.Array(&v.AllowedIPs), &v.RateLimit, &v.IsActive, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Service) DeleteAPIKey(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM api_keys WHERE id=$1`, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Service) UpdateAPIKey(ctx context.Context, id, name string, allowedIPs []string, rateLimit *int, sessionID string, active *bool) (APIKey, error) {
	var v APIKey
	err := s.db.QueryRowContext(ctx, `UPDATE api_keys SET name=$2, allowed_ips=$3, rate_limit=$4, session_id=NULLIF($5,''), is_active=COALESCE($6,is_active), updated_at=now() WHERE id=$1 RETURNING id,name,key_prefix,COALESCE(session_id,''),COALESCE(allowed_ips, ARRAY[]::text[]),rate_limit,is_active,created_at,updated_at`, id, name, allowedIPs, rateLimit, sessionID, active).Scan(&v.ID, &v.Name, &v.KeyPrefix, &v.SessionID, pq.Array(&v.AllowedIPs), &v.RateLimit, &v.IsActive, &v.CreatedAt, &v.UpdatedAt)
	return v, err
}

func (s *Service) Logout(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET refresh_token_hash=NULL, updated_at=now() WHERE id=$1`, userID)
	return err
}

func (s *Service) ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error {
	var stored string
	if err := s.db.QueryRowContext(ctx, `SELECT password_hash FROM users WHERE id=$1`, userID).Scan(&stored); err != nil {
		return ErrUnauthorized
	}
	if bcrypt.CompareHashAndPassword([]byte(stored), []byte(oldPassword)) != nil {
		return ErrUnauthorized
	}
	next, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE users SET password_hash=$2,refresh_token_hash=NULL,updated_at=now() WHERE id=$1`, userID, string(next))
	return err
}

func parse(token string, secret []byte) (jwt.MapClaims, error) {
	parsed, err := jwt.Parse(token, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, ErrUnauthorized
		}
		return secret, nil
	})
	if err != nil || !parsed.Valid {
		return nil, ErrUnauthorized
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	issuer, issuerOK := claims["iss"].(string)
	if !ok || !issuerOK || issuer != "wa-gateway" {
		return nil, ErrUnauthorized
	}
	return claims, nil
}

func hash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func GenerateAPIKey() string {
	return fmt.Sprintf("wag_%s", strings.TrimSpace(hex.EncodeToString(randomBytes(24))))
}
func randomBytes(n int) []byte { b := make([]byte, n); _, _ = rand.Read(b); return b }
