package main

import (
	"fmt"
	"os"
	"time"

	"github.com/mattn/go-tty"

	"git.tcp.direct/kayos/pxndscvm"
)

var swamp *pxndscvm.Swamp
var quit chan bool

func init() {
	quit = make(chan bool)
	swamp = pxndscvm.NewDefaultSwamp()
	// swamp.EnableDebug()
	if err := swamp.SetMaxWorkers(1000); err != nil {
		panic(err)
	}



	err := swamp.LoadProxyTXT("socks.list")
	if err != nil {
		println(err.Error())
		os.Exit(1)
	}
}

func main() {
	t, err := tty.Open()
	if err != nil {
		panic(err)
	}
	defer t.Close()

	go func() {
		for {
			r, err := t.ReadRune()
			if err != nil {
				panic(err)
			}
			switch string(r) {
			case "d":
				if swamp.DebugEnabled() {
					println("disabling debug")
					swamp.DisableDebug()
				} else {
					println("enabling debug")
					swamp.EnableDebug()
				}
			case "q":
				quit <- true
			default:
				time.Sleep(25 * time.Millisecond)
			}
		}
	}()

	for {
		select {
		case <-quit:
			return
		default:
			fmt.Printf("4: %d, 4a: %d, 5: %d \n", swamp.Stats.Valid4, swamp.Stats.Valid4a, swamp.Stats.Valid5)
			time.Sleep(1 * time.Second)
		}
	}
}