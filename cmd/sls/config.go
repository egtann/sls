package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

type config struct {
	RetainFor time.Duration
	Dir       string
	Port      string
	APIKey    string
}

func loadConfig(pth string) (*config, error) {
	fi, err := os.Open(pth)
	if err != nil {
		return nil, errors.Wrap(err, "open")
	}
	defer fi.Close()
	c := config{}
	scn := bufio.NewScanner(fi)
	for scn.Scan() {
		line := scn.Text()
		fields := strings.SplitN(line, "=", 2)
		if len(fields) != 2 {
			continue
		}
		key := strings.TrimSpace(fields[0])
		if len(key) == 0 {
			continue
		}
		if key[0] == '#' {
			continue
		}
		val := strings.TrimSpace(fields[1])
		if len(val) >= 2 {
			if val[0] == '"' || val[0] == '\'' {
				val = val[1 : len(val)-1]
			}
		}
		switch key {
		case "DIR":
			c.Dir = val
		case "PORT":
			if _, err = strconv.Atoi(val); err != nil {
				return nil, fmt.Errorf("%s PORT must be int", val)
			}
			c.Port = val
		case "RETAIN_FOR_DAYS":
			i, err := strconv.Atoi(val)
			if err != nil {
				return nil, fmt.Errorf("%s RETAIN_FOR_DAYS must be int", val)
			}
			c.RetainFor = time.Duration(i) * 24 * time.Hour
		case "API_KEY":
			c.APIKey = val
		default:
			return nil, fmt.Errorf("unknown config key: %s", key)
		}
	}
	if err = scn.Err(); err != nil {
		return nil, errors.Wrap(err, "scan")
	}
	errMsg := ""
	if c.APIKey == "" {
		errMsg += "missing API_KEY\n"
	}
	if c.RetainFor == time.Duration(0) {
		errMsg += "missing RETAIN_FOR_DAYS\n"
	}
	if c.Port == "" {
		errMsg += "missing PORT\n"
	}
	if c.Dir == "" {
		errMsg += "missing DIR\n"
	}
	if errMsg != "" {
		return nil, errors.New(errMsg)
	}
	return &c, nil
}
