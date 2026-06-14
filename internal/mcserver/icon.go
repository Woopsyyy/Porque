package mcserver

import (
	"bytes"
	"context"
	"image"
	_ "image/jpeg" // register JPEG decoder
	"image/png"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	xdraw "golang.org/x/image/draw"

	"github.com/woopsy/porque/internal/apperr"
)

// iconSize is the fixed dimension Minecraft requires for server-icon.png.
const iconSize = 64

// SetIcon decodes an uploaded image, center-crops it to a square, scales it to
// a clean 64×64 PNG, and writes it directly to the server folder.
func (c *Controller) SetIcon(ctx context.Context, serverID uuid.UUID, raw []byte) error {
	srv, err := c.store.GetServer(ctx, serverID)
	if err != nil {
		return err
	}

	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return apperr.Validation("unsupported image — use PNG or JPEG")
	}

	// Center-crop to a square so scaling doesn't distort the picture.
	b := src.Bounds()
	side := b.Dx()
	if b.Dy() < side {
		side = b.Dy()
	}
	x0 := b.Min.X + (b.Dx()-side)/2
	y0 := b.Min.Y + (b.Dy()-side)/2
	square := image.NewRGBA(image.Rect(0, 0, side, side))
	xdraw.Draw(square, square.Bounds(), src, image.Pt(x0, y0), xdraw.Src)

	// High-quality downscale to 64×64.
	dst := image.NewRGBA(image.Rect(0, 0, iconSize, iconSize))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), square, square.Bounds(), xdraw.Over, nil)

	iconPath := filepath.Join(srv.VolumeName, "server-icon.png")
	f, err := os.OpenFile(iconPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return apperr.Internal(err)
	}
	defer f.Close()

	if err := png.Encode(f, dst); err != nil {
		return apperr.Internal(err)
	}

	return nil
}

// GetIcon returns the current server-icon.png bytes, or NotFound when none.
func (c *Controller) GetIcon(ctx context.Context, serverID uuid.UUID) ([]byte, error) {
	srv, err := c.store.GetServer(ctx, serverID)
	if err != nil {
		return nil, err
	}

	iconPath := filepath.Join(srv.VolumeName, "server-icon.png")
	data, err := os.ReadFile(iconPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, apperr.NotFound("no icon set")
		}
		return nil, apperr.Internal(err)
	}

	return data, nil
}
