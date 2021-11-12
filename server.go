package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/miekg/dns"
	"log"
	"net"
	"strings"
	"sync"
)

type handler struct{}

type DockerCli struct {
	*client.Client
}

var dockerDomains map[string]string
var updateLock sync.Mutex
var serverMode bool
var listenAddr string
var domainSuffix string

func (_ *handler) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	msg := dns.Msg{}
	msg.SetReply(r)
	updateLock.Lock()
	defer updateLock.Unlock()
	// log.Printf("%v",r.Question[0])
	switch r.Question[0].Qtype {
	case dns.TypeA:
		msg.Authoritative = true
		domain := strings.TrimRight(msg.Question[0].Name,domainSuffix)
		address, ok := dockerDomains[domain]
		domain = dns.Fqdn(msg.Question[0].Name)
		if ok {
			msg.Answer = append(msg.Answer, &dns.A{
				Hdr: dns.RR_Header{Name: domain, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
				A:   net.ParseIP(address),
			})
		}
	}
	w.WriteMsg(&msg)
}

func (cli DockerCli) getNetworkName(containerID string) map[string]string {
	filter := filters.NewArgs()
	if containerID != "" {
		filter.Add("id", containerID)
	}
	containers, err := cli.ContainerList(context.Background(), types.ContainerListOptions{Filters: filter})
	if err != nil {
		log.Printf("error %v", err)
		return map[string]string{}
	}
	var nameSet = make(map[string]string, 0)
	for i := range containers {
		container, err := cli.ContainerInspect(context.Background(), containers[i].ID)
		if err != nil {
			log.Printf("error %v", err)
			continue
		}
		var IPAddress string

		for networkName := range container.NetworkSettings.Networks {
			network := container.NetworkSettings.Networks[networkName]
			IPAddress = network.IPAddress
			for _, alias := range network.Aliases {
				nameSet[alias] = IPAddress
			}
		}

		nameSet[container.ID[:12]] = IPAddress
		nameSet[container.Name[1:]] = IPAddress
	}
	updateLock.Lock()
	log.Printf("update domain map")
	dockerDomains = nameSet
	updateLock.Unlock()
	return nameSet
}

func (cli DockerCli) eventListener() {
	filter := filters.NewArgs()
	filter.Add("type", "container")
	filter.Add("event", "start")
	filter.Add("event", "unpause")
	filter.Add("event", "pause")
	filter.Add("event", "die")
	messageChan, errorChan := cli.Events(context.Background(), types.EventsOptions{Filters: filter})
	for {
		select {
		case msg := <-messageChan:
			switch msg.Action {
			case "start", "unpause":
				log.Printf("start %v",msg.Actor.ID[:12])
				cli.getNetworkName("")
			case "pause", "die":
				log.Printf("stop %v",msg.Actor.ID[:12])
				cli.getNetworkName("")
			}
		case err := <-errorChan:
			log.Fatal(err)
		}
	}
}

func printResult(nameToResolve string) {
	for k,v := range dockerDomains{
		if nameToResolve == "" || strings.Contains(k,nameToResolve){
			fmt.Printf("%s: %s\n", k,v)
		}
	}
}

func main() {
	flag.BoolVar(&serverMode,"d",false,"run as DNS server")
	flag.StringVar(&listenAddr,"l",":5300","address to listen, default :5300")
	flag.StringVar(&domainSuffix,"s","d.com","domain suffix, default d.com")
	flag.Parse()
	name := flag.Args()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	DockerCli{cli}.getNetworkName("")
	if serverMode {
		fmt.Printf("DNS server listening on %s\n",listenAddr)
		go DockerCli{cli}.eventListener()
		srv := &dns.Server{Addr: listenAddr, Net: "udp"}
		srv.Handler = &handler{}
		if err := srv.ListenAndServe(); err != nil {
			log.Fatalf("Failed to set udp listener %s\n", err.Error())
		}
	}
	if len(name) == 0 {
		printResult("")
	}else{
		printResult(name[0])
	}
}
