package main

import (
	//~ "fmt"
	"github.com/sakaisatoru/go_radio_raspi/mpvctl"
	"time"
)

type RadioVolume struct {
	volume       int8
	visible      bool
	visibleTimer *time.Timer
	visibleSpan  time.Duration
}

func RadioVolumeNew() *RadioVolume {
	v := RadioVolume{
		volume:      0,
		visible:     true,
		visibleSpan: 700 * time.Millisecond,
	}
	v.visibleTimer = time.AfterFunc(v.visibleSpan, func() {
		v.visible = false
	})
	return &v
}

func (v *RadioVolume) IsVisible() bool {
	return v.visible
}

func (v *RadioVolume) Set(n int8) {
	v.volume = n
}

func (v *RadioVolume) Get() int8 {
	return v.volume
}

//~ func (v *RadioVolume) Show() {
//~ mu.Lock()
//~ defer mu.Unlock()

//~ if displayInfo == displayInfoOnlyDoubleheightClock {
//~ return
//~ }

//~ s := fmt.Sprintf("vol:%2d   ", v.volume)
//~ oled.PrintWithPos(0, 1, []byte(s))

//~ if v.visible {
//~ v.visibleTimer.Stop() // 以前のスケジュールが生きていれば止める
//~ }
//~ v.visibleTimer.Reset(v.visibleSpan)
//~ v.visible = true
//~ }

func (v *RadioVolume) Increment() {
	v.volume++
	if v.volume > mpvctl.VolumeMax {
		v.volume = mpvctl.VolumeMax
	}
	mpvctl.Setvol(v.volume)
	//~ v.Show()
}

func (v *RadioVolume) Decrement() {
	v.volume--
	if v.volume <= mpvctl.VolumeMin {
		v.volume = mpvctl.VolumeMin
	}
	mpvctl.Setvol(v.volume)
	//~ v.Show()
}
