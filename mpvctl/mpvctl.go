package mpvctl

import (
		"fmt"
		"net"
		"time"
		"os/exec"
		"strings"
		"encoding/json"
)

type MpvIRC struct {
    Data       	string	 `json:"data"`
    Name		string	 `json:"name"`
	Request_id  int	 	 `json:"request_id"`
    Err 		string	 `json:"error"`
    Event		string	 `json:"event"`
}

func (ms *MpvIRC) clear() {
	ms.Data = "" 
    ms.Name = ""
	ms.Request_id = 0
    ms.Err = ""		
    ms.Event = ""		
}

const (
	IRCbuffsize int = 1024
	MPVOPTION1     string = "--idle"
	MPVOPTION2     string = "--input-ipc-server="
	MPVOPTION3     string = "--no-video"
	MPVOPTION4     string = "--no-cache"
	MPVOPTION5     string = "--stream-buffer-size=256KiB"
)

var (
	mpv net.Conn
	mpvprocess *exec.Cmd
	volconv = []int8{	0,1,2,3,4,4,5,6,6,7,7,8,8,9,9,10,10,11,11,
						11,12,12,13,13,13,14,14,14,15,15,16,16,16,17,
						17,17,18,18,18,19,19,20,20,20,21,21,22,22,23,
						23,24,24,25,25,26,26,27,27,28,28,29,30,30,31,
						32,32,33,34,35,35,36,37,38,39,40,41,42,43,45,
						46,47,49,50,52,53,55,57,59,61,63,66,68,71,74,
						78,81,85,90,95,100}
	Volume_min int8 = 0
	Volume_max int8 = int8(len(volconv) - 1)
	readbuf = make([]byte, IRCbuffsize)
	Cb_connect_stop = func() bool { return false } 
)

func Init(socketpath string) error {
	mpvprocess = exec.Command("/usr/bin/mpv", 	MPVOPTION1, 
												MPVOPTION2+socketpath, 
												MPVOPTION3, MPVOPTION4, 
												MPVOPTION5)
	err := mpvprocess.Start()
	return err
}

func Mpvkill() error {
	err := mpvprocess.Process.Kill()
	return err
}

func Open(socket_path string) error {
	var err error
	for i := 0; ;i++ {
		mpv, err = net.Dial("unix", socket_path)
		if err == nil {
			break
		}
		time.Sleep(200*time.Millisecond)
		if i > 60 {
			return err	// time out
		}
	}
	return nil
}

func Close() {
	mpv.Close()
}

func Send(s string) error {
	fmt.Printf("s = %s\n",s)
	_, err := mpv.Write([]byte(s))
	return err
}



func Recv(ch chan<- string, cb func(MpvIRC) (string, bool)) {
	var ms MpvIRC
	
	for {
		n, err := mpv.Read(readbuf)
		if err == nil {
			if n < IRCbuffsize {
				for _, s := range(strings.Split(string(readbuf[:n]),"\n")) {
					if len(s) > 0 {
						ms.clear() // 中身を消さないとフィールド単位で持ち越される場合がある
						err := json.Unmarshal([]byte(s),&ms)
						if err == nil {
							s, ok := cb(ms)
							if ok  {
								ch <- s
							}
						}
					}
				}
			}
		}
	}
}

func Setvol(vol int8) {
	if vol < Volume_min {
		vol = Volume_min
	} else if vol > Volume_max {
		vol = Volume_max
	} 
	s := fmt.Sprintf("{\"command\": [\"set_property\",\"volume\",%d]}\x0a",volconv[vol])
	Send(s)
}

func Stop() {
	if Cb_connect_stop() == false {
		Send("{\"command\": [\"stop\"]}\x0a")
	}
}

