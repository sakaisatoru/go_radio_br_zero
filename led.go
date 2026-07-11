package main

import (
	"github.com/stianeikeland/go-rpio/v4"
)

type Led struct {
}

func LedNew() *Led {
	return &Led{}
}

func (v *Led) GreenOn() {
	rpio.Pin(pinReLed2).High() // 赤 OFF
	rpio.Pin(pinReLed1).Low()  // 緑 ON
}

func (v *Led) GreenOff() {
	rpio.Pin(pinReLed1).High() // 緑 OFF
}

func (v *Led) RedOn() {
	rpio.Pin(pinReLed1).High() // 緑 OFF
	rpio.Pin(pinReLed2).Low()  // 赤 ON
}

func (v *Led) RedOff() {
	rpio.Pin(pinReLed2).High() // 赤 OFF
}

func (v *Led) YellowOn() {
	rpio.Pin(pinReLed1).Low() // 緑 ON
	rpio.Pin(pinReLed2).Low() // 赤 ON
}

func (v *Led) YellowOff() {
	rpio.Pin(pinReLed1).High() // 緑 OFF
	rpio.Pin(pinReLed2).High() // 赤 OFF
}

func (v *Led) ChangeColor(s StateCode) {
	switch s {
	case stateNormalMode, stateVolumeSet:
		v.GreenOn()
	case stateTuneMode:
		v.RedOn()
	case stateSelectFunction, stateAlarmHourSet, stateAlarmMinSet:
		v.YellowOn()
	}
}
