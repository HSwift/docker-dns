package main

import (
	"context"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/miekg/dns"
	"log"
	"net"
	"strconv"
	"sync"
)

type handler struct{}

type DockerCli struct {
	*client.Client
}

type DockerDomain struct {
	IPAddress string
	names     []string
}

var dockerDomains map[string]string
var updateLock sync.Mutex

func (_ *handler) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	msg := dns.Msg{}
	msg.SetReply(r)
	updateLock.Lock()
	defer updateLock.Unlock()
	log.Printf("%v",r.Question[0])
	switch r.Question[0].Qtype {
	case dns.TypeA:
		msg.Authoritative = true
		domain := msg.Question[0].Name
		address, ok := dockerDomains[domain]
		if ok {
			msg.Answer = append(msg.Answer, &dns.A{
				Hdr: dns.RR_Header{Name: domain, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
				A:   net.ParseIP(address),
			})
		}
	}
	w.WriteMsg(&msg)
}

func convertToMap(input []DockerDomain) {
	updateLock.Lock()
	defer updateLock.Unlock()
	log.Printf("update domain map")
	dockerDomains = make(map[string]string, 0)
	for _, domain := range input {
		for _, name := range domain.names {
			dockerDomains[dns.Fqdn(name+".d.com")] = domain.IPAddress
		}
	}
}

func (cli DockerCli) getNetworkName(containerID string) []DockerDomain {
	filter := filters.NewArgs()
	if containerID != "" {
		filter.Add("id", containerID)
	}
	containers, err := cli.ContainerList(context.Background(), types.ContainerListOptions{Filters: filter})
	if err != nil {
		log.Printf("error %v", err)
		return []DockerDomain{}
	}
	var results []DockerDomain = make([]DockerDomain, 0)
	for i := range containers {
		container, err := cli.ContainerInspect(context.Background(), containers[i].ID)
		if err != nil {
			log.Printf("error %v", err)
			continue
		}
		var nameSet = make(map[string]struct{}, 2)
		var IPAddress string
		nameSet[container.ID[:12]] = struct{}{}
		nameSet[container.Name[1:]] = struct{}{}
		for networkName := range container.NetworkSettings.Networks {
			network := container.NetworkSettings.Networks[networkName]
			IPAddress = network.IPAddress
			for _, alias := range network.Aliases {
				nameSet[alias] = struct{}{}
			}
		}
		var names []string = make([]string, len(nameSet))
		for k := range nameSet {
			names = append(names, k)
		}
		results = append(results, DockerDomain{IPAddress, names})
	}
	return results
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
				log.Printf("start %v",msg.ID[:12])
				convertToMap(cli.getNetworkName(""))
			case "pause", "die":
				log.Printf("stop %v",msg.ID[:12])
				convertToMap(cli.getNetworkName(""))
			}
		case err := <-errorChan:
			log.Fatal(err)
		}
	}
}

func main() {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	convertToMap(DockerCli{cli}.getNetworkName(""))
	go DockerCli{cli}.eventListener()
	srv := &dns.Server{Addr: ":" + strconv.Itoa(53), Net: "udp"}
	srv.Handler = &handler{}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Failed to set udp listener %s\n", err.Error())
	}
}
