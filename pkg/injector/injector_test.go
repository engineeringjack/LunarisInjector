package injector

import (
	"path/filepath"
	"testing"
)

func TestExtractGameDir(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			name:     "space separated",
			args:     []string{"-Xmx4G", "--username", "Steve", "--gameDir", "/home/user/mc", "--version", "1.20.1"},
			expected: filepath.Clean("/home/user/mc"),
		},
		{
			name:     "equals separated",
			args:     []string{"-Xmx4G", "--gameDir=/home/user/mc", "--version", "1.20.1"},
			expected: filepath.Clean("/home/user/mc"),
		},
		{
			name:     "jvm property",
			args:     []string{"-Dminecraft.applet.TargetDirectory=/home/user/mc", "net.minecraft.client.main.Main"},
			expected: filepath.Clean("/home/user/mc"),
		},
		{
			name:     "none",
			args:     []string{"-Xmx4G", "net.minecraft.client.main.Main"},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractGameDir(tt.args)
			if got != tt.expected {
				t.Errorf("ExtractGameDir() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestFindRealJava(t *testing.T) {
	// On this Linux machine, /usr/bin/java exists
	java, err := FindRealJava("")
	if err != nil {
		t.Fatalf("FindRealJava failed: %v", err)
	}
	if java == "" {
		t.Errorf("expected found Java path, got empty")
	}
}
