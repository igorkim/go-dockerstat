package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	dockType "github.com/docker/engine-api/types"
)

// const (
// 	modeImageName  = "image"
// 	modeNamePrefix = "prefix"
// 	modeSubstring  = "substring"
// modeRegex =  "regex"
// )

type NetStats map[string]NetStat

type NetStat struct {
	// Name string
	// Lifespan int64
	Count int
	RxBps int64
	TxBps int64
}

type OneStat struct {
	Name     string
	Lifespan int64
	RxBps    int64
	TxBps    int64
}

func main() {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}

	containers, err := cli.ContainerList(ctx, types.ContainerListOptions{})
	if err != nil {
		panic(err)
	}

	netstats := NetStats{}

	onestatchan := make(chan OneStat)
	for _, container := range containers {
		go func(container types.Container) {
			statsshot, err := cli.ContainerStatsOneShot(ctx, container.ID)
			if err != nil {
				panic(err)
			}
			defer statsshot.Body.Close()

			var stats dockType.StatsJSON
			decoder := json.NewDecoder(statsshot.Body)
			err = decoder.Decode(&stats)
			if err != nil {
				panic(err)
			}

			eth0 := stats.Networks["eth0"]
			lifespan := time.Now().Unix() - container.Created

			onestat := OneStat{
				Name:     container.Image,
				Lifespan: lifespan,
				RxBps:    int64(eth0.RxBytes) / lifespan,
				TxBps:    int64(eth0.TxBytes) / lifespan,
			}
			onestatchan <- onestat
			log.Printf("[%+v - %s - %d s - %d bps - %d bps]", container.Names, container.Image, lifespan, int64(eth0.RxBytes)/lifespan, int64(eth0.TxBytes)/lifespan)
		}(container)
	}

	for range containers {
		onestat := <-onestatchan

		netstat := netstats[onestat.Name]
		netstat.Count++
		netstat.RxBps += onestat.RxBps
		netstat.TxBps += onestat.TxBps
		netstats[onestat.Name] = netstat
	}

	images := make([]string, 0, len(netstats))
	for image := range netstats {
		images = append(images, image)
	}

	sort.Strings(images)

	for _, image := range images {
		netstat := netstats[image]
		log.Printf("%s - %d - %s Rx bps - %s Tx bps", image, netstat.Count, ByteCountIEC(netstat.RxBps), ByteCountIEC(netstat.TxBps))
	}
}

func ByteCountIEC(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB",
		float64(b)/float64(div), "KMGTPE"[exp])
}
