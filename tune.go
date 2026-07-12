package main

import (
	"github.com/sakaisatoru/go_mpvradio/netradio"
	"github.com/sakaisatoru/go_radio_raspi/mpvctl"
		"local.packages/volume"
	"log"
	"strings"
)

func tune() {
	var (
		stationURL string
		err        error = nil
	)

	// 選局に変更がなければ戻る
	if radioState.IsRadioEnable() && !radioState.IsCannelChange() {
		return
	}
	m := radioState.CurrentStationName()
	infomation.Update(0, m)

	args := strings.Split(radioState.CurrentStationURL(), "/")
	if args[0] == "plugin:" {
		switch args[1] {
		case "afn.py":
			stationURL, err = netradio.AFNGetUrlWithApi(args[2])
			if err != nil {
				log.Println(err)
				return
			}
		case "radiko.py":
			var err error
			for i := 0; i < 3; i++ {
				// エラーの際は認証トークンの期限切れを見越して２回再挑戦する
				err = radikoproxy.RadikoGetUrl(args[2])
				if err == nil {
					break
				}
			}
			if err != nil {
				log.Println(err)
				return
			}

			if radikoproxy.IsStop() {
				radikoproxy.Start()
			}
			stationURL = radikoproxy.GetProxyAddress()
		default:
			break
		}
	} else {
		stationURL = radioState.CurrentStationURL()
	}

	mpvctl.Setvol(volume.Get())
	mpvctl.Loadfile(stationURL)
	radioState.RadioEnable()
	radioState.CannelUpdate()
}
