package main

import (
	"log"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type Host struct {
	URL *url.URL

	IsAlive bool
	HCMux   sync.RWMutex
}

func (h *Host) HealthCheck() {
	log.Println("checking for the host", h.URL)

	timeout := 2 * time.Second
	conn, err := net.DialTimeout("tcp", h.URL.String(), timeout)
	if err != nil {
		log.Printf("Site: %s unreachable, error: %s ", h.URL.String(), err)

		// set the active flag to false
		h.HCMux.Lock()
		h.IsAlive = false
		h.HCMux.Unlock()
	}

	if conn != nil {
		conn.Close()
	}

	// set the active flag to true
	h.HCMux.Lock()
	h.IsAlive = true
	h.HCMux.Unlock()
}

type ServerPool struct {
	Hosts        map[string]*Host
	NodeMux      sync.RWMutex
	Current      *Host
	Exit         chan struct{}
	DelayedCheck time.Duration
}

func NewServerPool(hostsAddrs []string) *ServerPool {

	hostMap := make(map[string]*Host)

	for _, host := range hostsAddrs {
		// parse the address into url type
		url, err := url.Parse(host)
		if err != nil {
			log.Printf("%s is not a valid, err: %s", host, err)
		}

		hostMap[url.Host] = &Host{
			URL:     url,
			HCMux:   sync.RWMutex{},
			IsAlive: true,
		}

		hostMap[url.Host].HealthCheck()
	}

	return &ServerPool{
		Hosts:        hostMap,
		NodeMux:      sync.RWMutex{},
		Current:      nil,
		Exit:         make(chan struct{}),
		DelayedCheck: time.Duration(3) * time.Second,
	}
}

func (s *ServerPool) PassiveHealthCheck() {
	for {
		select {
		case <-s.Exit:
			return
		default:
			time.Sleep(s.DelayedCheck)
			for _, host := range s.Hosts {
				host.HealthCheck()
			}
		}
	}
}

func (s *ServerPool) SetNextPeer() *Host {
	if s.Current == nil || !s.Current.IsAlive {
		for _, host := range s.Hosts {
			if host.IsAlive {
				s.Current = host
			}
		}
	}
	return s.Current
}

func (s *ServerPool) Serve() {
	http.ListenAndServe(":3000", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.SetNextPeer()
		log.Panicln("redirect to: ", s.Current.URL)
		http.Redirect(w, r, s.Current.URL.Host, 200)
		http.Error(w, "Service not available", http.StatusServiceUnavailable)
	}))
}

func main() {

	readHosts := []string{"localhost:3002", "localhost:3001"}

	serverPool := NewServerPool(readHosts)

	go serverPool.PassiveHealthCheck()

	serverPool.Serve()
}
