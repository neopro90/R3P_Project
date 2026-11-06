package main

import "testing"

func TestPhotoContentTypeAllowsRasterOnly(t *testing.T) {
	tests := []struct {
		extension string
		want      string
		ok        bool
	}{
		{extension: ".jpg", want: "image/jpeg", ok: true},
		{extension: ".JPEG", want: "image/jpeg", ok: true},
		{extension: ".png", want: "image/png", ok: true},
		{extension: ".webp", want: "image/webp", ok: true},
		{extension: ".svg", ok: false},
		{extension: ".html", ok: false},
		{extension: ".pdf", ok: false},
	}
	for _, test := range tests {
		got, ok := photoContentType(test.extension)
		if got != test.want || ok != test.ok {
			t.Errorf("photoContentType(%q) = (%q, %t), want (%q, %t)", test.extension, got, ok, test.want, test.ok)
		}
	}
}
