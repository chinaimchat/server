package file

import "testing"

func TestMinioObjectPathForPreview(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"file/preview/chat/1/89365/1774486662648.jpg", "chat/1/89365/1774486662648.jpg"},
		{"file/preview/sticker/duck/cover.png", "file/file/preview/sticker/duck/cover.png"},
		{"chat/1/2/x.jpg", "chat/1/2/x.jpg"},
		{"file/preview/moment/abc/1.png", "moment/abc/1.png"},
		{"file/preview/unknown/foo/bar", "file/preview/unknown/foo/bar"},
	}
	for _, tc := range cases {
		if got := minioObjectPathForPreview(tc.in); got != tc.want {
			t.Errorf("minioObjectPathForPreview(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
