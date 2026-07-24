package rotaryencoder

import (
	"github.com/stianeikeland/go-rpio/v4"
	"time"
)

type REvector int

const (
	NoData REvector = iota
	Forward
	Backward
)

type RotaryEncoder struct {
	pinA         rpio.Pin
	pinB         rpio.Pin
	counter      int
	samplingtime int
	cbForward    func()
	cbBackward   func()
}

var (
	dir = [...]int{0, 1, -1, 0, -1, 0, 0, 1, 1, 0, 0, -1, 0, -1, 1, 0}
)

func cbDefault() {
}

func New(a rpio.Pin, b rpio.Pin, cbFor func(), cbBack func()) RotaryEncoder {
	if cbFor == nil {
		cbFor = cbDefault
	}
	if cbBack == nil {
		cbBack = cbDefault
	}
	return RotaryEncoder{
		pinA:         a,
		pinB:         b,
		counter:      0,
		samplingtime: 2,
		cbForward:    cbFor,
		cbBackward:   cbBack,
	}
}

func (r *RotaryEncoder) Init() {
	r.ResetCounter()
}

func (r *RotaryEncoder) ResetCounter() {
	r.counter = 0
}

func (r *RotaryEncoder) GetCounter() int {
	return r.counter
}

func (r *RotaryEncoder) SetCounter(n int) int {
	r.counter = n
	return r.counter
}

func (r *RotaryEncoder) SetSamplingTime(n int) int {
	r.samplingtime = n
	return r.samplingtime
}

// DetectLoop デテント型エンコーダ専用（1刻みで4相動く）
func (r *RotaryEncoder) DetectLoop(code chan<- REvector) {
	var (
		idx, current uint8
		store        int
	)
	for {
		time.Sleep(time.Duration(r.samplingtime) * time.Millisecond)
		current = uint8(r.pinA.Read())<<1 | uint8(r.pinB.Read())
		idx = (idx << 2) | current
		store += dir[idx&15]
		if current == 3 {
			switch {
			case store <= -4:
				store = 0
				r.counter++
				r.cbForward()
				code <- Forward
			case store >= 4:
				store = 0
				r.counter--
				r.cbBackward()
				code <- Backward
			default:
				store = 0 // チャタリングで数値が暴れたら消去
			}
		}
	}
}
