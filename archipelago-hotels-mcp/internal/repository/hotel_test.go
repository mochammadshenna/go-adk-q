package repository

import (
	"testing"
)

func TestResizeImageURL(t *testing.T) {
	tests := []struct {
		name     string
		img      string
		w, h     int
		location string
		want     string
	}{
		{
			name: "empty",
			img:  "",
			want: "",
		},
		{
			name:     "canonical ADR example",
			img:      "https://storage.astonwebsite.com/images/hotel/thumb.jpg",
			location: "center",
			want:     "https://images.archipelagohotels.com/astonwebsite/images/hotel/thumb.jpg",
		},
		{
			name:     "sentineltech direct domain",
			img:      "https://sentineltech.com/uploads/thumb.jpg",
			location: "center",
			want:     "https://images.archipelagohotels.com/sentineltech-publicwebsite/uploads/thumb.jpg",
		},
		{
			name:     "sentineltech S3 URL",
			img:      "https://sentineltech.s3.amazonaws.com/hotels/thumb.jpg",
			location: "center",
			want:     "https://images.archipelagohotels.com/sentineltech-publicwebsite/hotels/thumb.jpg",
		},
		{
			name:     "www prefix",
			img:      "https://www.astonwebsite.com/path/img.jpg",
			location: "center",
			want:     "https://images.archipelagohotels.com/astonwebsite/path/img.jpg",
		},
		{
			name:     "bare domain no subdomain",
			img:      "https://neohotels.com/images/thumb.jpg",
			location: "center",
			want:     "https://images.archipelagohotels.com/neohotels/images/thumb.jpg",
		},
		{
			name:     "with dimensions",
			img:      "https://storage.astonwebsite.com/thumb.jpg",
			w:        800,
			h:        600,
			location: "center",
			want:     "https://images.archipelagohotels.com/astonwebsite/thumb.jpg?d=800x600&location=center",
		},
		{
			name:     "width only",
			img:      "https://storage.astonwebsite.com/thumb.jpg",
			w:        400,
			location: "top",
			want:     "https://images.archipelagohotels.com/astonwebsite/thumb.jpg?s=400&location=top",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resizeImageURL(tt.img, tt.w, tt.h, tt.location)
			if got != tt.want {
				t.Errorf("resizeImageURL(%q, %d, %d, %q)\n  got  %q\n  want %q",
					tt.img, tt.w, tt.h, tt.location, got, tt.want)
			}
		})
	}
}
