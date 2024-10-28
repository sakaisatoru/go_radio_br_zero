module ss1200.lomo.jp/go_radio_br_zero

go 1.22.1

replace local.packages/aqm0802a => ./aqm0802a

require (
	github.com/davecheney/i2c v0.0.0-20140823063045-caf08501bef2
	github.com/sakaisatoru/go_radio_raspi/mpvctl v0.0.0-20241028134331-40808f1af03d
	github.com/sakaisatoru/go_radio_raspi/netradio v0.0.0-20241028134331-40808f1af03d
	github.com/sakaisatoru/go_radio_raspi/rotaryencoder v0.0.0-20241028134331-40808f1af03d
	github.com/stianeikeland/go-rpio/v4 v4.6.0
	local.packages/aqm0802a v0.0.0-00010101000000-000000000000
)

require (
	github.com/carlmjohnson/requests v0.23.5 // indirect
	golang.org/x/net v0.15.0 // indirect
)
