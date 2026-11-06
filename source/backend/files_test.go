package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCleanStorageRelativePath(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "root", input: "", want: "."},
		{name: "root dot", input: ".", want: "."},
		{name: "nested path", input: "photos/2026", want: "photos/2026"},
		{name: "parent segment", input: "../etc", wantErr: true},
		{name: "embedded parent segment", input: "photos/../etc", wantErr: true},
		{name: "absolute path", input: "/etc/passwd", wantErr: true},
		{name: "windows separator", input: `photos\private`, wantErr: true},
		{name: "empty segment", input: "photos//private", wantErr: true},
		{name: "null byte", input: "photo\x00.jpg", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := cleanStorageRelativePath(test.input)
			if (err != nil) != test.wantErr {
				t.Fatalf("cleanStorageRelativePath(%q) error = %v, wantErr %v", test.input, err, test.wantErr)
			}
			if err == nil && got != test.want {
				t.Fatalf("cleanStorageRelativePath(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

func TestUSBMountValidation(t *testing.T) {
	mounts := []string{
		"/dev/sda1 /mnt/storage ext4 rw,relatime 0 0",
		"/dev/sdb1 /media/USB\\040Drive ext4 rw,relatime 0 0",
		"/dev/loop0 /mnt/test ext4 rw,relatime 0 0",
		"/dev/sdb1 /media/archive ext4 rw,relatime 0 0",
	}

	if !usbMountExists("/mnt/storage", mounts) {
		t.Fatal("expected USB block device mount to be accepted")
	}
	if !usbMountExists("/media/archive", mounts) {
		t.Fatal("expected removable media mount to be accepted")
	}
	if !usbMountExists("/media/USB Drive", mounts) {
		t.Fatal("expected escaped mount point to be decoded")
	}
	if usbMountExists("/mnt/test", mounts) {
		t.Fatal("loop device must not be accepted as USB storage")
	}
	if usbMountExists("/etc", mounts) {
		t.Fatal("non-mounted system directory must not be accepted")
	}
	if isUSBStorageMount("/dev/sda1", "/") {
		t.Fatal("filesystem root must not be accepted")
	}
}

func TestListStorageDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "Photos"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "readme.txt"), []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}

	entries, truncated, err := listStorageDirectory(root, ".")
	if err != nil {
		t.Fatal(err)
	}
	if truncated {
		t.Fatal("unexpected truncated directory")
	}

	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0].Name != "Photos" || !entries[0].Directory {
		t.Fatalf("directory should sort first: %+v", entries[0])
	}
	if entries[1].Name != "readme.txt" || entries[1].Directory || entries[1].Size != 5 {
		t.Fatalf("unexpected file entry: %+v", entries[1])
	}

	if _, _, err := listStorageDirectory(root, "../"); err == nil {
		t.Fatal("parent traversal path must fail")
	}
}

func TestListStorageDirectoryCapsEntries(t *testing.T) {
	root := t.TempDir()
	for i := 0; i <= maxStorageDirectoryEntries; i++ {
		name := filepath.Join(root, "file-"+string(rune('a'+i%26))+string(rune('A'+i/26)))
		if err := os.WriteFile(name, nil, 0600); err != nil {
			t.Fatal(err)
		}
	}

	entries, truncated, err := listStorageDirectory(root, ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != maxStorageDirectoryEntries || !truncated {
		t.Fatalf("got %d entries, truncated=%t; want %d and true", len(entries), truncated, maxStorageDirectoryEntries)
	}
}

func TestListStorageDirectorySkipsSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "outside-link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}

	entries, _, err := listStorageDirectory(root, ".")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(entries, []storageFileEntry{}) {
		t.Fatalf("symlink should not be listed: %+v", entries)
	}
}
