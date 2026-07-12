package queue

import "testing"

func TestMessageTypeRoutingContract(t *testing.T) {
	for _, typ := range []string{"text", "image", "document", "pdf"} {
		if !isSupportedMessageType(typ) {
			t.Errorf("%s should be supported", typ)
		}
	}
	for _, typ := range []string{"image", "document", "pdf"} {
		if !isMediaMessageType(typ) {
			t.Errorf("%s should be media", typ)
		}
	}
	if isSupportedMessageType("video") || isMediaMessageType("text") {
		t.Fatal("message type routing contract is incorrect")
	}
}
