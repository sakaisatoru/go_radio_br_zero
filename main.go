package main

import (
	"github.com/davecheney/i2c"
	"github.com/sakaisatoru/go_mpvradio/netradio"
	"github.com/sakaisatoru/go_radio_raspi/mpvctl"
	"github.com/sakaisatoru/go_radio_raspi/rotaryencoder"
	"github.com/stianeikeland/go-rpio/v4"
	"local.packages/aqm0802a"
	"local.packages/volume"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

const (
	stationListFile string = "/home/sakai/program/radio.m3u"
	MpvSocketPath   string = "/run/mpvsocket"
	Version         string = "ﾗｼﾞｵv2.x"
)

type ButtonCode int

const (
	BtnStationNone       ButtonCode = iota
	BtnStationReForward             // rotaryencoder.Forward compatible
	BtnStationReBackward            // rotaryencoder.Backward compatible
	BtnStationReButton
	BtnStationReButtonLong
	BtnStationReButtonRepeat
	btnSystemShutdown

	BtnStationRepeat = 0x80

	btnPressWidth     int = 5
	btnPressLongWidth int = 25
)

const (
	ErrorHup = iota
	ErrorMpvConn
	ErrorMpvFault
	Space8
	ErrorTuning
	ErrorRpioNotOpen
	ErrorSocketNotOpen
)

const (
	pinReButton = 3
	pinReA      = 19
	pinReB      = 26

	pinAfAmp        = 12
	pinLcdReset     = 17
	pinLcdBacklight = 4
	pinReLed1       = 5
	pinReLed2       = 6
)

var (
	mpv         net.Conn
	mu          sync.Mutex
	lcd         *aqm0802a.AQM0802A
	radikoproxy *netradio.RadikoProxy
	radioState  *RadioState
	infomation *InfomationDisplay
	colon      uint8
	errmessage = [...]string{
		"HUP     ",   // HUP
		"mpv ｴﾗｰ  ",  //
		"mpv ﾌｫﾙﾄ ",  //
		"        ",   // Space8
		"tuneｴﾗｰ  ",  //
		"rpioｴﾗｰ  ",  //
		"ｿｹｯﾄｴﾗｰ   ", //
	}

	jst      *time.Location = time.FixedZone("JST", 9*60*60)
	voltable                = []int8{0, 15, 20, 25, 31, 37, 43, 49, 57, 63, 68}
)

func btninput(code chan<- ButtonCode) {
	hold := 0
	btn_h := BtnStationNone

	for {
		time.Sleep(10 * time.Millisecond)

		if btn_h == 0 {
			if rpio.Pin(pinReButton).Read() == rpio.Low {
				// 押されているボタンがあれば、そのコードを保存する
				btn_h = BtnStationReButton
				hold = 0
			}
		} else {
			// もし過去に押されていたら、現在それがどうなっているか調べる
			if rpio.Pin(pinReButton).Read() == rpio.Low {
				// 引き続き押されている
				hold++
				if hold > btnPressLongWidth {
					hold--
					//~ time.Sleep(100*time.Millisecond)// リピート幅調整用
					lcd.OneShotLight()
					code <- BtnStationReButtonRepeat // リピート入力
				}
			} else {
				if hold >= btnPressLongWidth {
					code <- BtnStationReButtonLong // リピート入力の終わり(ボタン長押し)
				} else if hold > btnPressWidth {
					lcd.OneShotLight()
					code <- btn_h // ワンショット入力
				}
				btn_h = 0
				hold = 0
			}
		}
	}
}

func afamp_enable() {
	rpio.Pin(pinAfAmp).High()
}

func afampDisable() {
	rpio.Pin(pinAfAmp).Low()
}

func shutdown() {
	infomation.Update(0, "shutdown")
	time.Sleep(700 * time.Millisecond)
	cmd := exec.Command("/sbin/poweroff", "")
	cmd.Start()
	afampDisable() // AF amp disable
	lcd.DisplayOff()
	//~ i2c.Close()
	lcd.LightOff()
}

func main() {
	// GPIO initialize
	var firsterror error
	for i := 0; i < 15; i++ {
		firsterror = rpio.Open()
		if firsterror == nil {
			break
		}
		if os.IsNotExist(firsterror) {
			log.Println(firsterror)
			time.Sleep(2 * time.Second)
		}
	}
	if firsterror != nil {
		log.Println("exit program")
		return
	}
	defer rpio.Close()
	for _, sn := range []rpio.Pin{pinReButton, pinReA, pinReB} {
		sn.Input()
		sn.PullUp()
	}
	for _, sn := range []rpio.Pin{pinAfAmp, pinLcdReset,
		pinLcdBacklight, pinReLed1, pinReLed2} {
		sn.Output()
		sn.PullUp()
		sn.Low()
	}

	// I2C LCD 初期化
	i2c, err := i2c.New(0x3e, 0) // aqm0802a
	if err != nil {
		log.Println(firsterror)
		return
	}
	defer i2c.Close()

	// LCD表示器向け表示ルーチン
	infomation = InfomationDisplayNew()

	lcd = aqm0802a.New(i2c, pinLcdReset, pinLcdBacklight)
	lcd.Init()
	infomation.Update(0, Version)
	lcd.OneShotLight()

	// rotaryencoder
	rencoder := rotaryencoder.New(pinReB, pinReA,
		lcd.OneShotLight, lcd.OneShotLight)
	//~ rencoder.SetSamplingTime(4)

	// 受信時の状態遷移管理
	radioState = RadioStateNew()

	// mpv
	err = mpvctl.Init(MpvSocketPath)
	if err != nil {
		infomation.ShowError(ErrorMpvFault)
		log.Println(err)
		return
	}
	mpvctl.SetVoltable(&voltable)

	// mpvctl.Stop() のコールバック関数
	mpvctl.Cb_connect_stop = func() bool {
		infomation.ShowError(Space8)
		afampDisable() // AF amp disable
		radioState.RadioDisable()
		return false
	}

	// 音量調整
	volume.Set(mpvctl.VolumeMax / 3)

	// シグナルハンドラ
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGQUIT,
		syscall.SIGHUP, syscall.SIGINT)

	// 局リストの準備
	if err := radioState.ReadStationListInfo(stationListFile); err != nil {
		infomation.ShowError(ErrorHup)
		log.Println(err)
		return
	}

	// radiko用代理サーバー
	radikoproxy = netradio.RadikoProxyNew()

	// mpv socket
	if mpvctl.Open() != nil {
		infomation.ShowError(ErrorMpvConn)
		log.Println(err) // time out
		return
	}
	defer func() {
		mpvctl.Close()
		if err := mpvctl.Mpvkill(); err != nil {
			log.Println(err)
		}
	}()

	mpvret := make(chan string)
	// mpvからの応答を選別するフィルタ
	go mpvctl.Recv(mpvret, func(ms mpvctl.MpvIRC) (string, bool) {
		//~ fmt.Printf("%#v\n",ms)
		if radioState.IsRadioEnable() {
			if ms.Event == "property-change" {
				if ms.Name == "metadata/by-key/icy-title" {
					return ms.Data, true
				}
			}
		}
		return "", false
	})

	mpvctl.Setvol(volume.Get())
	s := "{ \"command\": [\"observe_property_string\", 1, \"metadata/by-key/icy-title\"] }"
	mpvctl.Send(s)
	colon = 0

	colonblink := time.NewTicker(500 * time.Millisecond)

	// 入力受付起動
	btncode := make(chan ButtonCode)
	go btninput(btncode)
	btnREcode := make(chan rotaryencoder.REvector)
	go rencoder.DetectLoop(btnREcode)

	radioState.GreenOn()
	for {
		select {
		case <-signals:
			if err = os.Remove(MpvSocketPath); err != nil {
				log.Println(err)
			}
			afampDisable() // AF amp disable
			lcd.DisplayOff()
			i2c.Close()
			lcd.LightOff()
			signal.Stop(signals) // close()だとpanicする事がある、らしい
			return

		case title := <-mpvret:
			// mpv の応答でフィルタで処理された文字列をここで処理する
			stmp := radioState.CurrentStationName()
			if title != "" {
				stmp = stmp + "  " + title
			}
			infomation.Update(0, stmp)

		case <-colonblink.C:
			colon ^= 1
			infomation.ShowClock(radioState.GetStateString(colon))
			radioState.TokeiCheck()

		case r := <-btnREcode:
			radioState.Dispatch(ButtonCode(r))

		case r := <-btncode:
			radioState.Dispatch(r)
		}
	}
}
