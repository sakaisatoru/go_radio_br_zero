package aqm0802a

import (
	"github.com/davecheney/i2c"
	"time"
	"fmt"
)

const (
	i2c_SLAVE = 0x0000
)

type Config struct {
}

type AQM0802A struct {
	bus		i2c.I2C
	Config	Config
}

func New(bus *i2c.I2C) AQM0802A {
	return AQM0802A {
		bus:		*bus,
	}
}

func (d *AQM0802A) Configure() {
	// LCD
	time.Sleep(40 * time.Millisecond)                 // power on 後の推奨待ち時間

	init := []byte{0x38, 0x39, 0x14, 0x70, 0x56, 0x6c, 0x38, 0x01, 0x0c}
	for _, r := range init {
		_, err := d.bus.Write([]byte{0x00, r})
		if r == 0x6c {
			// Fllower control 後の処理
			time.Sleep(300*time.Millisecond)
		} else {
			time.Sleep(27*time.Microsecond)
		}
		//~ fmt.Printf("%02X ",r)
		if err != nil {
			fmt.Printf("%02X ",r);
			fmt.Println(err)
		}
	}
	time.Sleep(2*time.Millisecond)
}

func (d *AQM0802A) ConfigureWithSettings(config Config) {
}

func (d *AQM0802A) Init() {
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


