package config

import (
	"filecoin-spade-client/pkg/log"
	"fmt"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/mcuadros/go-defaults"
	"os"
	"strings"
	"time"
)

type Configuration struct {
	DownloadPath        string `default:"/tmp/filecoin-spade-downloads"`
	MaxSpadeDealsActive int    `default:"20"`
	InsecureSkipVerify  bool   `default:"false"`

	LotusConfig LotusConfig
	SpadeConfig SpadeConfig
	BoostConfig BoostConfig
}

type SpadeConfig struct {
	Url                    string        `default:"https://api.spade.storacha.network/"`
	PendingRefreshInterval time.Duration `default:"30s"`
}

type LotusConfig struct {
	DaemonUrl       string `default:"127.0.0.1:1234"`
	DaemonAuthToken string `default:"undefined"`

	MinerUrl       string `default:"127.0.0.1:2345"`
	MinerAuthToken string `default:"undefined"`
}

type BoostConfig struct {
	BoostUrl       string `default:"127.0.0.1:3456"`
	BoostAuthToken string `default:"undefined"`
	GraphQlPort    int    `default:"8080"`
	GraphQlUrl     string `default:""`
}

func NewDefaultConfiguration() Configuration {
	config := new(Configuration)
	defaults.SetDefaults(config)

	info := cliutil.ParseApiInfo(os.Getenv("FULLNODE_API_INFO"))
	daemonUrl, err := info.DialArgs("v1")
	if err != nil {
		log.Fatalf("could not parse FULLNODE_API_INFO: %s", err)
	}

	config.LotusConfig.DaemonUrl = daemonUrl
	config.LotusConfig.DaemonAuthToken = string(info.Token)

	minerInfo := cliutil.ParseApiInfo(os.Getenv("MINER_API_INFO"))
	minerPath, err := minerInfo.DialArgs("v0")
	if err != nil {
		log.Fatalf("could not parse MINER_API_INFO: %s", err)
	}

	config.LotusConfig.MinerUrl = minerPath
	config.LotusConfig.MinerAuthToken = string(minerInfo.Token)

	marketInfo := cliutil.ParseApiInfo(os.Getenv("MARKETS_API_INFO"))
	marketPath, err := marketInfo.DialArgs("v0")
	if err != nil {
		log.Fatalf("could not parse MARKETS_API_INFO: %s", err)
	}

	config.BoostConfig.BoostUrl = marketPath
	parsedHost, err := marketInfo.Host()
	if err != nil {
		log.Fatalf("could not parse MARKETS_API_INFO: %s", err)
	}
	config.BoostConfig.GraphQlUrl = fmt.Sprintf("http://%s:%d", strings.Split(parsedHost, ":")[0], config.BoostConfig.GraphQlPort)
	config.BoostConfig.BoostAuthToken = string(marketInfo.Token)

	return *config
}
