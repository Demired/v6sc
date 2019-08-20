package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"
)

func checkDomain(site Site) {
	ns, err := net.LookupHost(site.Domain)
	if err != nil || len(ns) < 1 {
		ch <- site
		<-task
		return
	}
	var protocols = []string{"http://", "https://"}
	for _, s := range ns {
		for _, p := range protocols {
			var v int
			if net.ParseIP(s).To4() != nil {
				v = 4
				site.IPv4 = s
			} else {
				v = 6
				site.IPv6 = s
			}
			if resp, e := protocol(site.Domain, v, p); e == nil {
				var protoMajor = resp.ProtoMajor
				if p == "http://" {
					if v == 4 {
						site.V4hp = 2
					} else if v == 6 {
						var zero time.Time
						if zero == site.V6time {
							site.V6time = time.Now()
						}
						site.V6hp = 2
					}
				}
				if p == "https://" {
					if v == 4 {
						if protoMajor == 2 {
							site.V4h2 = 2
						}
						site.V4hs = 2
					} else if v == 6 {
						var zero time.Time
						if zero == site.V6time {
							site.V6time = time.Now()
						}
						if protoMajor == 2 {
							site.V6h2 = 2
						}
						site.V6hs = 2
					}
				}
				var expirationTime time.Time
				if resp.TLS != nil {
					for _, tls := range resp.TLS.PeerCertificates {
						if expirationTime.IsZero() {
							expirationTime = tls.NotAfter
						}
						if expirationTime.Before(tls.NotAfter) == false {
							expirationTime = tls.NotAfter
						}
					}
				}
				site.CETime = expirationTime
			}
		}
	}
	s, _ := json.Marshal(site)
	log.Printf("task finish：%s", s)
	ch <- site
	<-task
}

func protocol(domain string, v int, p string) (*http.Response, error) {
	var client *http.Client
	if v == 4 {
		client = &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					network = "tcp4" //仅使用ipv4
					return net.Dial(network, addr)
				},
			},
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Timeout: time.Second * 15,
		}
	} else {
		client = &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					network = "tcp6" //仅使用ipv6
					return net.Dial(network, addr)
				},
			},
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Timeout: time.Second * 15,
		}
	}
	var url = fmt.Sprintf("%s%s", p, domain)
	req, _ := http.NewRequest("HEAD", url, nil)
	resp, e := client.Do(req)
	if e != nil {
		return nil, errors.New("fail")
	}

	var expirationTime time.Time

	if resp.TLS != nil {
		for _, tls := range resp.TLS.PeerCertificates {
			if expirationTime.IsZero() {
				expirationTime = tls.NotAfter
			}
			if expirationTime.Before(tls.NotAfter) == false {
				expirationTime = tls.NotAfter
			}
		}
	}

	return resp, nil
}
