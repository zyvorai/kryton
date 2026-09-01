package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/zyvorai/kryton/internal/auth"
)

type client struct {
	base, token, project string
	http                 *http.Client
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	if os.Args[1] == "hash-token" {
		if len(os.Args) != 3 {
			fmt.Fprintln(os.Stderr, "usage: krytonctl hash-token TOKEN")
			os.Exit(2)
		}
		fmt.Println(auth.HashToken(os.Args[2]))
		return
	}
	if os.Args[1] == "generate-token" {
		var b [32]byte
		if _, err := rand.Read(b[:]); err != nil {
			panic(err)
		}
		fmt.Println(base64.RawURLEncoding.EncodeToString(b[:]))
		return
	}
	c := client{base: strings.TrimRight(env("KRYTON_URL", "http://localhost:8080"), "/"), token: os.Getenv("KRYTON_TOKEN"), project: env("KRYTON_PROJECT", "default"), http: &http.Client{Timeout: 30 * time.Second}}
	switch os.Args[1] {
	case "list":
		c.do("GET", "/api/v1/machines?project="+c.project, nil)
	case "images":
		c.do("GET", "/api/v1/images", nil)
	case "events":
		c.do("GET", "/api/v1/events?limit=50", nil)
	case "capabilities":
		c.do("GET", "/api/v1/capabilities", nil)
	case "doctor":
		c.do("GET", "/api/v1/doctor", nil)
	case "storage":
		c.do("GET", "/api/v1/storage", nil)
	case "set-storage":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "set-storage requires STORAGE_CLASS (or \"\" to clear)")
			os.Exit(2)
		}
		c.do("PUT", "/api/v1/storage/config", map[string]any{"storageClass": os.Args[2]})
	case "get", "start", "stop", "delete", "snapshot", "snapshots", "restore", "delete-snapshot":
		machineCommand(c, os.Args[1], os.Args[2:])
	case "create":
		createCommand(c, os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func createCommand(c client, args []string) {
	f := flag.NewFlagSet("create", flag.ExitOnError)
	image := f.String("image", "windows-server-2025", "image id")
	cpu := f.Int("cpu", 4, "vCPU")
	memory := f.Int("memory", 8192, "memory MiB")
	disk := f.Int("disk", 80, "boot disk GiB")
	network := f.String("network", "", "Multus network attachment definition")
	ttl := f.Int("ttl", 0, "TTL in minutes")
	project := f.String("project", c.project, "project")
	_ = f.Parse(args)
	if f.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "create requires NAME")
		os.Exit(2)
	}
	body := map[string]any{"project": *project, "name": f.Arg(0), "image": *image, "compute": map[string]any{"cpu": *cpu, "memoryMiB": *memory}, "disk": map[string]any{"sizeGiB": *disk}, "network": map[string]any{"networkId": *network}, "ttlMinutes": *ttl}
	c.do("POST", "/api/v1/machines", body)
}
func machineCommand(c client, cmd string, args []string) {
	f := flag.NewFlagSet(cmd, flag.ExitOnError)
	project := f.String("project", c.project, "project")
	name := f.String("name", "", "snapshot name")
	_ = f.Parse(args)
	if f.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "%s requires MACHINE_ID\n", cmd)
		os.Exit(2)
	}
	id := f.Arg(0)
	q := "?project=" + *project
	switch cmd {
	case "get":
		c.do("GET", "/api/v1/machines/"+id+q, nil)
	case "start", "stop":
		c.do("POST", "/api/v1/machines/"+id+"/"+cmd+q, nil)
	case "delete":
		c.do("DELETE", "/api/v1/machines/"+id+q, nil)
	case "snapshot":
		c.do("POST", "/api/v1/machines/"+id+"/snapshot"+q, map[string]any{"name": *name})
	case "snapshots":
		c.do("GET", "/api/v1/machines/"+id+"/snapshots"+q, nil)
	case "restore":
		if f.NArg() != 2 {
			fmt.Fprintln(os.Stderr, "restore requires MACHINE_ID SNAPSHOT_ID")
			os.Exit(2)
		}
		c.do("POST", "/api/v1/machines/"+id+"/snapshots/"+f.Arg(1)+"/restore"+q, nil)
	case "delete-snapshot":
		if f.NArg() != 2 {
			fmt.Fprintln(os.Stderr, "delete-snapshot requires MACHINE_ID SNAPSHOT_ID")
			os.Exit(2)
		}
		c.do("DELETE", "/api/v1/machines/"+id+"/snapshots/"+f.Arg(1)+q, nil)
	}
}
func (c client) do(method, path string, body any) {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, r)
	if err != nil {
		fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, err := c.http.Do(req)
	if err != nil {
		fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	var pretty bytes.Buffer
	if json.Indent(&pretty, b, "", "  ") == nil {
		fmt.Println(pretty.String())
	} else {
		fmt.Print(string(b))
	}
	if res.StatusCode >= 400 {
		os.Exit(1)
	}
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
func usage() {
	fmt.Println(`krytonctl commands:
  list
  create NAME [--image ID --cpu N --memory MiB --disk GiB --network NAD --ttl MIN]
  get MACHINE_ID
  start MACHINE_ID
  stop MACHINE_ID
  snapshot MACHINE_ID [--name NAME]
  snapshots MACHINE_ID
  restore MACHINE_ID SNAPSHOT_ID
  delete-snapshot MACHINE_ID SNAPSHOT_ID
  delete MACHINE_ID
  images
  events
  capabilities
  doctor
  storage
  set-storage STORAGE_CLASS
  generate-token
  hash-token TOKEN

Environment: KRYTON_URL, KRYTON_TOKEN, KRYTON_PROJECT`)
}
