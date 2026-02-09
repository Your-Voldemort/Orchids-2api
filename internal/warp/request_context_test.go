package warp

import (
	"strings"
	"testing"
)

func TestBuildRequestBytes_UsesProvidedWorkdir(t *testing.T) {
	t.Parallel()

	reqBytes, err := buildRequestBytes("check cwd", "auto", nil, nil, false, "/Users/dailin/Documents/GitHub/Orchids-2api", "")
	if err != nil {
		t.Fatalf("buildRequestBytes: %v", err)
	}

	s := string(reqBytes)
	if !strings.Contains(s, "/Users/dailin/Documents/GitHub/Orchids-2api") {
		t.Fatalf("expected dynamic workdir in request bytes")
	}
	if strings.Contains(s, "/Users/lofyer") {
		t.Fatalf("unexpected hardcoded /Users/lofyer in request bytes")
	}
}
