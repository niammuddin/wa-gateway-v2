package queue

import "testing"

func TestQueueNameByPriority(t *testing.T) {
	tests := []struct {
		priority int
		want     string
	}{
		{priority: 10, want: "messages_high"},
		{priority: 0, want: "messages"},
		{priority: -1, want: "messages_low"},
	}
	for _, tt := range tests {
		if got := queueName(tt.priority); got != tt.want {
			t.Errorf("queueName(%d) = %q, want %q", tt.priority, got, tt.want)
		}
	}
}
