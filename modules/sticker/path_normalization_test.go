package sticker

import "testing"

func TestNormalizeStickerAPIPath(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"file/preview/sticker/duck/1.lim", "file/preview/sticker/duck/1.lim"},
		{"file/file/preview/sticker/duck/1.lim", "file/preview/sticker/duck/1.lim"},
		{"/file/preview/sticker/duck/1.lim", "file/preview/sticker/duck/1.lim"},
		{"preview/sticker/duck/1.lim", "file/preview/sticker/duck/1.lim"},
		{"sticker/duck/1.lim", "file/preview/sticker/duck/1.lim"},
		{"https://x/y/z", "https://x/y/z"},
		{"http://x/y", "http://x/y"},
		{"avatar/default/1.png", "avatar/default/1.png"},
	}
	for _, tc := range cases {
		if got := normalizeStickerAPIPath(tc.in); got != tc.want {
			t.Errorf("normalizeStickerAPIPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeStickerFormat(t *testing.T) {
	cases := []struct {
		path, format, want string
	}{
		{"file/preview/sticker/a/1.lim", "gzip", "lim"},
		{"file/preview/sticker/a/1.lim", "wrong", "lim"},
		{"sticker/a/1.LIM", "", "lim"},
		{"https://x.com/b/c/2.tgs?k=1", "gif", "tgs"},
		{"file/preview/sticker/a/x.gif", "lim", "gif"},
		{"file/preview/sticker/a/cover.webp", "", "webp"},
		{"file/preview/sticker/a/c.png", "gzip", "png"},
		{"file/preview/sticker/a/nosuffix", "gzip", "lim"},
		{"file/preview/sticker/a/nosuffix", "GZIP", "lim"},
		{"file/preview/sticker/a/nosuffix", "lottie", "lim"},
		{"file/preview/sticker/a/nosuffix", "rlottie", "lim"},
		{"file/preview/sticker/a/x", "application/x-gzip", "lim"},
		{"file/preview/sticker/a/x", "telegram-sticker", "tgs"},
		{"", "gzip", "lim"},
	}
	for _, tc := range cases {
		if got := normalizeStickerFormat(tc.path, tc.format); got != tc.want {
			t.Errorf("normalizeStickerFormat(%q,%q) = %q, want %q", tc.path, tc.format, got, tc.want)
		}
	}
}
