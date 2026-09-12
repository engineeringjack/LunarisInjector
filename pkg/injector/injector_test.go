package injector

import (
	"os"
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

func TestResolveRealBinary(t *testing.T) {
	tmpDir := t.TempDir()
	fakeJava := filepath.Join(tmpDir, "java")
	fakeJavaReal := filepath.Join(tmpDir, "java.real")

	_ = os.WriteFile(fakeJava, []byte("fake-wrapper"), 0755)
	_ = os.WriteFile(fakeJavaReal, []byte("fake-real"), 0755)

	if !HasSiblingRealJava(fakeJava) {
		t.Errorf("expected HasSiblingRealJava to be true for %s", fakeJava)
	}

	resolved := ResolveRealBinary(fakeJava)
	if resolved != fakeJavaReal {
		t.Errorf("expected ResolveRealBinary(%s) = %s, got %s", fakeJava, fakeJavaReal, resolved)
	}

	// For a binary without .real sibling
	standalone := filepath.Join(tmpDir, "standalone")
	_ = os.WriteFile(standalone, []byte("code"), 0755)
	if HasSiblingRealJava(standalone) {
		t.Errorf("expected HasSiblingRealJava to be false for %s", standalone)
	}
	if ResolveRealBinary(standalone) != standalone {
		t.Errorf("expected ResolveRealBinary(%s) = %s, got %s", standalone, standalone, ResolveRealBinary(standalone))
	}
}
