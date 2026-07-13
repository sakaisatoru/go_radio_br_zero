package volume

import (
	"github.com/sakaisatoru/go_radio_raspi/mpvctl"
	"time"
)

var (
	volume      int8          = 0
	visible     bool          = true
	visibleSpan time.Duration = 700 * time.Millisecond
)

func IsVisible() bool {
	return visible
}

func Set(n int8) {
	volume = n
}

func Get() int8 {
	return volume
}

func Increment() {
	volume++
	if volume > mpvctl.VolumeMax {
		volume = mpvctl.VolumeMax
	}
	mpvctl.Setvol(volume)
}

func Decrement() {
	volume--
	if volume <= mpvctl.VolumeMin {
		volume = mpvctl.VolumeMin
	}
	mpvctl.Setvol(volume)
}
