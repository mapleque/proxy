package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

type opType string

const (
	opTypeUnknown opType = "unknown"
	opTypeAdd            = "add"
	opTypeDel            = "del"
)

type Pod struct {
	Type   string `json:"type"`
	Object *struct {
		Metadata *struct {
			Name        string            `json:"name"`
			Labels      map[string]string `json:"labels"`
			Annotations map[string]string `json:"annotations"`
			Dt          string            `json:"deletionTimestamp"`
		} `json:"metadata"`
		Status *struct {
			Phase string `json:"phase"`
			Ip    string `json:"podIP"`
		} `json:"status"`
	} `json:"object"`

	op opType
	domain,
	ip string
	ports map[string]string

	waitforDel bool
}

func (w *watcher) NewPod(raw []byte) (*Pod, error) {
	this := &Pod{}
	if err := json.Unmarshal(raw, this); err != nil {
		return nil, err
	}

	// parse op
	this.op = opTypeUnknown
	switch {
	case this.Object == nil ||
		this.Object.Metadata == nil ||
		this.Object.Metadata.Name == "" ||
		this.Object.Status == nil:

		// type unknown
		// 避免空指针panic
	case this.Object.Status.Phase == "Running" &&
		this.Object.Metadata.Dt == "" &&
		this.Object.Status.Ip != "":

		this.ip = this.Object.Status.Ip
		w.podPool[this.Object.Metadata.Name] = this.Object.Status.Ip
		this.op = opTypeAdd
	case this.Type == "DELETED" &&
		this.Object.Metadata.Dt != "":

		if ip, ok := w.podPool[this.Object.Metadata.Name]; ok {
			this.ip = ip
			this.op = opTypeDel
		}
	}

	this.ports = map[string]string{}
	portPrefix := "port_"
	for k, v := range this.Object.Metadata.Labels {
		switch {
		case strings.HasPrefix(k, portPrefix):
			p := strings.ToLower(strings.TrimPrefix(k, portPrefix))
			this.ports[p] = v
		case k == "domain":
			this.domain = v
		default:
			// ignore
		}
	}
	if this.domain == "" {
		this.op = opTypeUnknown
	}

	return this, nil
}

func (this *Pod) String() string {
	return fmt.Sprintf(
		"\nop:%s\ndomain:%s\n%s\n%s",
		this.op,
		this.domain,
		func() string {
			if len(this.ports) == 0 {
				return "no port to proxy"
			}
			ret := []string{}
			for p, v := range this.ports {
				ret = append(ret, fmt.Sprintf("%s->%s", p, v))
			}
			return "proxy port:" + strings.Join(ret, ",")
		}(),
		func() string {
			bt, _ := json.MarshalIndent(this, "", "\t")
			return string(bt)
		}(),
	)
}

func (this *Pod) GetAppconfigFilename() string {
	return fmt.Sprintf("%s.json", this.domain)
}

func (this *Pod) GetDomainOnPort(portName string) string {
	ret := this.domain
	if portName != "" {
		ret = portName + "." + ret
	}
	return ret
}

func (this *Pod) GetServiceNameOnPort(portName string) string {
	ret := this.domain
	if portName != "" {
		ret += "." + portName
	}
	return strings.Replace(ret, ".", "_", -1)
}

func (this *Pod) GetPodHostOnPort(port string) string {
	return fmt.Sprintf("%s:%s", this.ip, port)
}
