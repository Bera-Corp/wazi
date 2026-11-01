//go:build linux

package probe

import "testing"

func TestStorageMountSelectionUsesContainingFilesystem(t *testing.T) {
	info := []byte("1 0 8:1 / / rw - ext4 /dev/root rw\n2 1 7:27 / /srv/wazi-runtime rw - ext4 /dev/loop27 rw\n3 1 7:28 / /srv/wazi-runtime-other rw - ext4 /dev/loop28 rw\n4 1 7:29 / /space\\040mount rw - ext4 /dev/loop29 rw\n")
	for _, test := range []struct{ path, want string }{
		{"/srv/wazi-runtime/home/.local/share/docker", "/srv/wazi-runtime"},
		{"/srv/wazi-runtime-other/docker", "/srv/wazi-runtime-other"},
		{"/srv/wazi-runtime-suffix/docker", "/"},
		{"/space mount/docker", "/space mount"},
	} {
		got, err := mountpointForPath(test.path, info)
		if err != nil || got != test.want {
			t.Fatalf("%s: got %s, %v", test.path, got, err)
		}
	}
	if _, err := mountpointForPath("/srv/docker", nil); err == nil {
		t.Fatal("missing mount evidence accepted")
	}
	if _, err := mountpointForPath("relative/docker", info); err == nil {
		t.Fatal("relative storage path accepted")
	}
}
