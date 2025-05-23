package aqm0802a

import (
	"github.com/davecheney/i2c"
	"github.com/stianeikeland/go-rpio/v4"
	"time"
	"fmt"
	"sync"
)

const (
	i2c_SLAVE = 0x0000
)

type Config struct {
}

type AQM0802A struct {
	bus				i2c.I2C
	pin_reset		int
	pin_backlight	int
	light			bool
	mu 				sync.Mutex
	Config	Config
}

var (
// ラテン１補助 (Latin-1-Supplement) の１部分 C2A0 - C2BF
	t_C2A0 = [...]byte{0x3F, 0xe9, 0xe4, 0xe5, 0x3F, 0xe6, 0x7c, 0x3F,
						0xf1, 0x3F, 0x61, 0xfb, 0x3F, 0x3F, 0x3F, 0xff,
						0xdf, 0x3F, 0x32, 0x33, 0xf4, 0x75, 0x3F, 0xa5,
						0x3F, 0x31, 0x30, 0xfc, 0xf6, 0xf5, 0x3F, 0x9f}

// ラテン１補助 (Latin-1-Supplement) の１部分 C380 - C3BF
	t_C380 = [...]byte{  0x41, 0x41, 0x8f, 0xea, 0x8e, 0x41, 0x92, 0x80,
						 0x45, 0x90, 0x45, 0x45, 0x49, 0x49, 0x49, 0x49,
						 
						 0x44, 0x4e, 0x4f, 0x4f, 0x4f, 0xec, 0x4f, 0xf7,
						 0xee, 0x55, 0x55, 0x55, 0x9a, 0x59, 0x3F, 0x3F,
						 
						 0x85, 0xe0, 0x83, 0xeb, 0x84, 0x61, 0x91, 0x87,
						 0x8a, 0x82, 0x88, 0x89, 0x8d, 0xe1, 0x8c, 0x8b,
						 
						 0x64, 0x9b, 0x95, 0xe2, 0x93, 0xed, 0x94, 0xf8,
						 0xee, 0x97, 0xe3, 0x96, 0x81, 0x79, 0x3F, 0x79}
// ギリシア文字
	t_CE90 = [...]byte{  0x3f, 0x41, 0x42, 0x09, 0x15, 0x45, 0x5a, 0x48,
						 0x16, 0x49, 0x4b, 0x17, 0x4d, 0x4e, 0x18, 0x4f,

						 0x19, 0x50, 0x3f, 0x1a, 0x54, 0x59, 0xef, 0x58, 
						 0x1d, 0x1e, 0x3f, 0x3f, 0x3f, 0x3f, 0x3f, 0x3f}
		
// 半角・全角形 (Halfwidth and Fullwidth Forms) の１部分 半角カナ EFBDA0 - EFBDBF
	t_EFBDA0 = [...]byte{0x20, 0xA1, 0xA2, 0xA3, 0xA4, 0xA5, 0xA6, 0xA7,
						 0xA8, 0xA9, 0xAA, 0xAB, 0xAC, 0xAD, 0xAE, 0xAF,
						 0xB0, 0xB1, 0xB2, 0xB3, 0xB4, 0xB5, 0xB6, 0xB7,
						 0xB8, 0xB9, 0xBA, 0xBB, 0xBC, 0xBD, 0xBE, 0xBF}

// 半角・全角形 (Halfwidth and Fullwidth Forms) の１部分 半角カナ EFBE80 - EFBE9F
	t_EFBE80 = [...]byte{0xC0, 0xC1, 0xC2, 0xC3, 0xC4, 0xC5, 0xC6, 0xC7,
						 0xC8, 0xC9, 0xCA, 0xCB, 0xCC, 0xCD, 0xCE, 0xCF,
						 0xD0, 0xD1, 0xD2, 0xD3, 0xD4, 0xD5, 0xD6, 0xD7,
						 0xD8, 0xD9, 0xDA, 0xDB, 0xDC, 0xDD, 0xDE, 0xDF}

)

func (d *AQM0802A) UTF8toOLED(s *[]byte) int {
	var	rv	[]byte
	rv = *s
	l := len(rv)
	pos := 0
	pass_count := 0
	for i, v := range rv {
		if pass_count > 0 {
			pass_count--
			continue
		}
		if v >= 0x20 && v <= 0x7f {
			rv[pos] = v
			pos++
			continue
		} 
		
		switch {
			case v == 0xc2:
				if l <= i + 1 {
					v = 0x3f
				} else if rv[i+1] >= 0xa0 && rv[i+1] <= 0xbf {
					pass_count = 1
					v = t_C2A0[rv[i+1]-0xa0]
				} else {
					pass_count = 1
					v = 0x3f	// '?'
				}
				
			case v == 0xc3:
				if l <= i + 1 {
					v = 0x3f
				} else if rv[i+1] >= 0x80 && rv[i+1] <= 0xbf {
					pass_count = 1
					v = t_C380[rv[i+1]-0x80]
				} else {
					pass_count = 1
					v = 0x3f	// '?'
				}

			case v == 0xce:
				if l <= i + 1 {
					v = 0x3f
				} else if rv[i+1] >= 0x90 && rv[i+1] <= 0xaf {
					pass_count = 1
					v = t_CE90[rv[i+1]-0x90]
				} else {
					pass_count = 1
					v = 0x3f
				}
				
			case v >= 0xc4 && v <= 0xdf:
				pass_count = 1
				v = 0x3f
				
			case v == 0xef:
				if l <= i + 2 {
					v = 0x3f
				} else if rv[i+1] == 0xbd {
					if rv[i+2] >= 0xa0 && rv[i+2] <= 0xbf {
						v = t_EFBDA0[rv[i+2]-0xa0]
					} else {
						v = 0x3f
					}
					pass_count = 2
				} else if rv[i+1] == 0xbe {
					if rv[i+2] >= 0x80 && rv[i+2] <= 0x9f {
						v = t_EFBE80[rv[i+2]-0x80]
					} else {
						v = 0x3f
					}
					pass_count = 2
				}
			
			case v >= 0xe0 && v <= 0xee:
				pass_count = 2
				v = 0x3f
				
			case v >= 0xf0 && v <= 0xf4:
				pass_count = 3
				v = 0x3f
		}
		rv[pos] = v
		pos++
	}
	return pos
}

func New(bus *i2c.I2C, reset_pin int, backlight_pin int) AQM0802A {
	return AQM0802A {
		bus:			*bus,
		pin_reset:		reset_pin,
		pin_backlight:	backlight_pin,
		light:			false,
	}
}

func (d *AQM0802A) Init() {
	// st7032.pdf p33
	d.Reset()
	time.Sleep(50 * time.Millisecond) // power on 後の推奨待ち時間 40mS以上

	//~ init := []byte{0x38, 0x39, 0x14, 0x70, 0x56, 0x6c, 0x38, 0x01, 0x0c}
	init := []byte{0x38, 0x39, 0x14, 0x70, 0x56, 0x6c, 0x0c, 0x01}
	for i := range init {
		_, err := d.bus.Write([]byte{0x00, init[i]})
		if init[i] == 0x6c {
			// Fllower control 後の処理
			time.Sleep(300*time.Millisecond) // > 200mS
		} else {
			time.Sleep(28*time.Microsecond) // > 26.3uS
		}
		if err != nil {
			fmt.Printf("%02X ",init[i])
			fmt.Println(err)
		}
	}
	time.Sleep(2*time.Millisecond)
}

func (d *AQM0802A) ConfigureWithSettings(config Config) {
}

func (d *AQM0802A) LightOn() {
	d.mu.Lock()
	defer d.mu.Unlock()
	rpio.Pin(d.pin_backlight).High()
	d.light = true
}

func (d *AQM0802A) LightOff() {
	d.mu.Lock()
	defer d.mu.Unlock()
	rpio.Pin(d.pin_backlight).Low()
	d.light = false
}

func (d *AQM0802A) IsLightOn() bool {
	return d.light
}

func (d *AQM0802A) Reset() {
	// st7032.pdf p47
	rpio.Pin(d.pin_reset).Low()
	time.Sleep(150*time.Microsecond) // tl > 100uS
	rpio.Pin(d.pin_reset).High()
}

func (d *AQM0802A) Configure() {
}

func (d *AQM0802A) Clear() {
	d.bus.Write([]byte{0x00, 0x01}) // Clear Display
	time.Sleep(1 * time.Millisecond)
	d.bus.Write([]byte{0x00, 0x02}) // Return Home
	time.Sleep(1 * time.Millisecond)
}

func (d *AQM0802A) DisplayOff() {
	d.bus.Write([]byte{0x00, 0x08}) // display off
	time.Sleep(1 * time.Millisecond)
}

func (d *AQM0802A) DisplayOn() {
	d.bus.Write([]byte{0x00, 0x0c}) // display on
	time.Sleep(1 * time.Millisecond)
}

func (d *AQM0802A) PrintWithPos(x uint8, y uint8, s []byte) {
	x &= 0x0f
	y &= 0x01
	//~ d.bus.Write([]byte{0x00, 0x80 + y*40 + x}) // set DDRAM address (aqm1602Y-NLW-FBW)
	d.bus.Write([]byte{0x00, 0x80 + y*0x40 + x}) // set DDRAM address (aqm0802A-NLW-GBW)
	time.Sleep(30 * time.Microsecond)

	d.bus.Write(append([]byte{0x40}, s...))
}


