package main

import (
	"time"
	"strings"
	"github.com/elastic/libbeat/logp"
	"github.com/parnurzeal/gorequest"
	"fmt"
)

type Poller struct {
	done          chan struct{}
	httpbeat      *Httpbeat
	config        UrlConfig
	period        time.Duration
}

func NewSpooler(httpbeat *Httpbeat, config UrlConfig) *Poller {
	poller := &Poller{
		httpbeat: httpbeat,
		config:   config,
	}

	return poller
}

func (p *Poller) Run() {
	//init the period
	if p.config.Period != nil {
		p.period = time.Duration(*p.config.Period) * time.Second
	} else {
		p.period = 1 * time.Second
	}

	ticker := time.NewTicker(p.period)
	defer ticker.Stop()

	// Loops until running is set to false
	for {
		select {
		case <-p.done:
		case <-ticker.C:
		}

		timerStart := time.Now()
		p.runOneTime()
		timerEnd := time.Now()

		duration := timerEnd.Sub(timerStart)
		if duration.Nanoseconds() > p.period.Nanoseconds() {
			logp.Warn("Ignoring tick(s) due to processing taking longer than one period")
		}
	}
}

func (p *Poller) runOneTime() error {
	request := gorequest.New()
	url := p.config.Url
	method := p.config.Method

	switch method {
	case "get":
		request.Get(url)
	case "delete":
		request.Delete(url)
	case "head":
		request.Head(url)
	case "patch":
		request.Patch(url)
	case "post":
		request.Post(url)
	case "put":
		request.Put(url)
	default:
		return fmt.Errorf("Unsupported HTTP method %g", method)
	}

	// set timeout
	if p.config.Timeout != nil {
		request.Timeout(time.Duration(*p.config.Timeout) * time.Second)
	} else {
		request.Timeout(60 * time.Second)
	}

	// set authentication
	if p.config.Username != "" && p.config.Password != "" {
		request.BasicAuth.Username = p.config.Username
		request.BasicAuth.Password = p.config.Password
	}

	// set body
	if p.config.Body !="" {
		request.SendString(p.config.Body)
	}

	// set headers
	request.Header = p.config.Headers

	// set proxy
	proxyUrl := p.GetProxyUrl()
	if proxyUrl != "" {
		request.Proxy(proxyUrl)
	}

	logp.Debug("Httpbeat", "Trying to make the following HTTP request: %v", request)
	now := time.Now()
	resp, body, errs:= request.End();

	if errs != nil {
		logp.Err("An error occured while executing HTTP request: %v", errs)
		return fmt.Errorf("An error occured while executing HTTP request: %v", errs)
	}

	requestEvent := Request{
		Url: url,
		Method: method,
		Headers: p.config.Headers,
		Body: p.config.Body,
	}

	responseEvent := Response{
		StatusCode:    resp.StatusCode,
		Headers:       p.GetResponseHeader(resp),
		Body:          body,
	}

	event := HttpEvent{
		ReadTime:     now,
		DocumentType: "httpbeat",
		Request:      requestEvent,
		Response:     responseEvent,
	}

	p.httpbeat.events.PublishEvent(event.ToMapStr())

	return nil
}

func (p *Poller) GetProxyUrl() (string) {
	proxyUrl := ""
	if p.config.ProxyHost != "" && p.config.ProxyPort != "" {
		proxyUrl = p.config.ProxyHost + p.config.ProxyPort
	}

	return proxyUrl;
}

func (p *Poller) Stop() {
	close(p.done)
}

func (p *Poller) GetResponseHeader(response gorequest.Response) map[string]string {

	responseHeader := make(map[string]string)
	for k,v := range response.Header {
		value := ""
		for _,h := range v {
			value+=h+" ,"
		}
		value = strings.TrimRight(value, " ,")
		responseHeader[k] = value
	}
	return responseHeader
}