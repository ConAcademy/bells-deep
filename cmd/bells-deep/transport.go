package main

import (
	"fmt"
	"github.com/NimbleMarkets/ntcharts/v2/picture"
	"os"
	"strings"
)

func kittyMedium(name string, env func(string) string, shared bool) (picture.KittyMedium, error) {
	switch name {
	case "direct":
		return picture.KittyMediumDirect, nil
	case "shm":
		return picture.KittyMediumSharedMemory, nil
	case "auto":
		if !shared || env("SSH_CONNECTION") != "" || env("SSH_CLIENT") != "" || env("SSH_TTY") != "" {
			return picture.KittyMediumDirect, nil
		}
		term, program := strings.ToLower(env("TERM")), strings.ToLower(env("TERM_PROGRAM"))
		if term == "xterm-ghostty" || term == "xterm-kitty" || program == "ghostty" || program == "kitty" || env("KITTY_WINDOW_ID") != "" {
			return picture.KittyMediumSharedMemory, nil
		}
		return picture.KittyMediumDirect, nil
	default:
		return 0, fmt.Errorf("--kitty-transport must be auto, direct, or shm")
	}
}
func configurePictures(m *model, transport, format string) error {
	medium, err := kittyMedium(transport, os.Getenv, picture.KittySharedMemorySupported())
	if err != nil {
		return err
	}
	encoding := picture.KittyFormatPNG
	switch format {
	case "png":
	case "rgba":
		encoding = picture.KittyFormatRGBA
	default:
		return fmt.Errorf("--kitty-format must be png or rgba")
	}
	// No image exists at startup, so these setters have no pending commands.
	m.pic.SetKittyMedium(medium)
	m.galleryPic.SetKittyMedium(medium)
	m.pic.SetKittyFormat(encoding)
	m.galleryPic.SetKittyFormat(encoding)
	return nil
}
