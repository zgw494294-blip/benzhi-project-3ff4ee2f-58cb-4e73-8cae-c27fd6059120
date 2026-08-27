package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

type config struct {
	addr      string
	database  string
	selfcheck bool
}

func parseConfig(args []string) (config, error) {
	set := flag.NewFlagSet("strata-proof", flag.ContinueOnError)
	addr := set.String("addr", "", "监听地址，必须为回环地址")
	database := set.String("db", "strata-proof.db", "SQLite 数据库路径")
	selfcheck := set.Bool("selfcheck", false, "通过真实回环 HTTP 执行完整自检后退出")
	if err := set.Parse(args); err != nil {
		return config{}, err
	}
	resolved := strings.TrimSpace(*addr)
	if resolved == "" {
		if port := strings.TrimSpace(os.Getenv("PORT")); port != "" {
			value, err := strconv.Atoi(port)
			if err != nil || value < 1 || value > 65535 {
				return config{}, fmt.Errorf("PORT 必须是 1 到 65535 的端口号")
			}
			resolved = net.JoinHostPort("127.0.0.1", port)
		} else {
			resolved = "127.0.0.1:19081"
		}
	}
	if err := validateAddress(resolved); err != nil {
		return config{}, err
	}
	if strings.TrimSpace(*database) == "" {
		return config{}, fmt.Errorf("数据库路径不能为空")
	}
	if set.NArg() != 0 {
		return config{}, fmt.Errorf("不支持位置参数")
	}
	return config{addr: resolved, database: *database, selfcheck: *selfcheck}, nil
}

func validateAddress(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("-addr 必须是 host:port: %w", err)
	}
	value, err := strconv.Atoi(port)
	if err != nil || value < 1 || value > 65535 {
		return fmt.Errorf("监听端口无效")
	}
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return fmt.Errorf("监听地址必须使用回环主机，拒绝 %q", host)
	}
	return nil
}
