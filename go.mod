module ss1200.lomo.jp/go_radio_br_zero

go 1.22.1

replace local.packages/netradio => ./netradio

replace local.packages/aqm0802a => ./aqm0802a

replace local.packages/mpvctl => ./mpvctl

require (
	github.com/davecheney/i2c v0.0.0-20140823063045-caf08501bef2
	github.com/stianeikeland/go-rpio/v4 v4.6.0
	local.packages/aqm0802a v0.0.0-00010101000000-000000000000
	local.packages/mpvctl v0.0.0-00010101000000-000000000000
	local.packages/netradio v0.0.0-00010101000000-000000000000
)

require (
	github.com/carlmjohnson/requests v0.23.5 // indirect
	golang.org/x/net v0.15.0 // indirect
)
