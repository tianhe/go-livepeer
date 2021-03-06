package main

import (
	"bufio"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"gopkg.in/urfave/cli.v1"
)

func main() {
	app := cli.NewApp()
	app.Name = "livepeer-cli"
	app.Usage = "interact with local Livepeer node"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "http",
			Usage: "local http port",
			Value: "8935",
		},
		cli.StringFlag{
			Name:  "rtmp",
			Usage: "local rtmp port",
			Value: "1935",
		},
		cli.StringFlag{
			Name:  "host",
			Usage: "host for the Livepeer node",
			Value: "localhost",
		},
		cli.IntFlag{
			Name:  "loglevel",
			Value: 4,
			Usage: "log level to emit to the screen",
		},
	}
	app.Action = func(c *cli.Context) error {
		// Set up the logger to print everything and the random generator
		log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(c.Int("loglevel")), log.StreamHandler(os.Stdout, log.TerminalFormat(true))))
		rand.Seed(time.Now().UnixNano())

		// Start the wizard and relinquish control
		w := &wizard{
			endpoint: fmt.Sprintf("http://%v:%v/status", c.String("host"), c.String("http")),
			rtmpPort: c.String("rtmp"),
			httpPort: c.String("http"),
			host:     c.String("host"),
			in:       bufio.NewReader(os.Stdin),
		}
		w.run()

		return nil
	}
	app.Run(os.Args)
}

type wizard struct {
	endpoint string // Local livepeer node
	rtmpPort string
	httpPort string
	host     string
	in       *bufio.Reader // Wrapper around stdin to allow reading user input
}

func (w *wizard) run() {
	// Make sure there is a local node running
	_, err := http.Get(w.endpoint)
	if err != nil {
		log.Error(fmt.Sprintf("Cannot find local node. Is your node running on http:%v and rtmp:%v?", w.httpPort, w.rtmpPort))
		return
	}

	fmt.Println("+-----------------------------------------------------------+")
	fmt.Println("| Welcome to livepeer-cli, your Livepeer command line tool  |")
	fmt.Println("|                                                           |")
	fmt.Println("| This tool lets you interact with a local Livepeer node    |")
	fmt.Println("| and participate in the Livepeer protocol without the	    |")
	fmt.Println("| hassle that it would normally entail.                     |")
	fmt.Println("|                                                           |")
	fmt.Println("+-----------------------------------------------------------+")
	fmt.Println()

	w.stats(false)
	// Basics done, loop ad infinitum about what to do
	for {
		fmt.Println()
		fmt.Println("What would you like to do? (default = stats)")
		fmt.Println(" 1. Get node status")
		fmt.Println(" 2. Deposit token")
		fmt.Println(" 3. Broadcast video")
		fmt.Println(" 4. Stream video")
		fmt.Println(" 5. Set transcoder config")
		fmt.Println(" 6. Set broadcast config")
		fmt.Println(" 7. Bond")
		fmt.Println(" 8. Unbond")
		fmt.Println(" 9. Withdraw bond")
		fmt.Println(" 10. Become a transcoder")
		fmt.Println(" 11. Get test Livepeer Token")
		fmt.Println(" 12. Get test Ether")
		fmt.Println(" 13. List registered transcoders")

		choice := w.read()
		switch {
		case choice == "" || choice == "1":
			w.stats(false)
		case choice == "2":
			w.deposit()
		case choice == "3":
			w.broadcast()
		case choice == "4":
			w.stream()
		case choice == "5":
			w.setTranscoderConfig()
		case choice == "6":
			w.setBroadcastConfig()
		case choice == "7":
			w.bond()
		case choice == "8":
			w.unbond()
		case choice == "9":
			w.withdrawBond()
		case choice == "10":
			w.activateTranscoder()
		case choice == "11":
			w.requestTokens()
		case choice == "12":
			fmt.Print("Go to eth-testnet.livepeer.org and use the faucet. (enter to continue)")
			w.read()
		case choice == "13":
			w.allTranscoderStats()
		default:
			log.Error("That's not something I can do")
		}
	}
}
