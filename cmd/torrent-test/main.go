package main

import (
	"github.com/CortexFoundation/CortexTheseus/log"
	"github.com/CortexFoundation/CortexTheseus/torrentfs"
	"github.com/anacrolix/torrent/metainfo"
	cli "gopkg.in/urfave/cli.v1"
	glog "log"
	"io/ioutil"
	"os"
	"os/signal"
	"syscall"
	"strings"
)

type Config struct {
	Dir        string
	TaskList   string
	LogLevel   int
	Utp        bool
}

var gitCommit = "" // Git SHA1 commit hash of the release (set via linker flags)

func main() {
	var conf Config
	app := cli.NewApp()

	app.Flags = []cli.Flag{
		cli.IntFlag{
			Name:        "verbosity",
			Value:       3,
			Usage:       "verbose level",
			Destination: &conf.LogLevel,
		},
  	cli.StringFlag{
			Name:        "dir",
			Value:       "data",
			Usage:       "datadir",
			Destination: &conf.Dir,
		},
  	cli.StringFlag{
			Name:        "task",
			Value:       "task",
			Usage:       "task list",
			Destination: &conf.TaskList,
		},
  	cli.BoolFlag{
			Name:        "utp",
			Usage:       "utp",
			Destination: &conf.Utp,
		},
	}

	app.Action = func(c *cli.Context) error {
		mainExitCode(&conf)
		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		glog.Fatal(err)
	}
}

func mainExitCode(conf *Config) int {
	log.Root().SetHandler(
		log.LvlFilterHandler(log.Lvl(conf.LogLevel), 
		log.StreamHandler(os.Stdout, log.TerminalFormat(true))),
	)

	cfg := torrentfs.Config{
		RpcURI:          "",
		DefaultTrackers: torrentfs.DefaultConfig.DefaultTrackers,
		SyncMode:        torrentfs.DefaultConfig.SyncMode,
		DisableUTP:      torrentfs.DefaultConfig.DisableUTP,
	}

	cfg.DataDir = conf.Dir
	cfg.DisableUTP = conf.Utp

	tm := torrentfs.NewTorrentManager(&cfg)
	tm.Start()

	if contents, err := ioutil.ReadFile(conf.TaskList); err == nil {
		tasks := strings.Split(string(contents), "\n")
		for _, task := range tasks {
			if len(task) != 40 {
				continue
			}
			log.Info("Task added", "task", task)
			tm.NewTorrent(torrentfs.FlowControlMeta{
				InfoHash: metainfo.NewHashFromHex(task),
				BytesRequested: 10000000,
			})
		}	
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	for {
		<-c
		tm.Close()
	}
	return 0
}
