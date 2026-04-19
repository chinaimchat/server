package util

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// GetExternalIP 获取本服务器的外网IP
func GetExternalIP() (string, error) {
	resp, err := http.Get("https://ipw.cn/api/ip/myip")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	resultBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(resultBytes)), nil
}

// GetClientPublicIP 尽最大努力实现获取客户端公网 IP 的算法。
// 解析 X-Real-IP 和 X-Forwarded-For 以便于反向代理（nginx 或 haproxy）可以正常工作。
func GetClientPublicIP(r *http.Request) string {
	var ip string
	for _, ip = range strings.Split(r.Header.Get("X-Forwarded-For"), ",") {
		ip = strings.TrimSpace(ip)
		if ip != "" {
			return ip
		}
	}
	ip = strings.TrimSpace(r.Header.Get("X-Real-Ip"))
	if ip != "" {
		return ip
	}
	if ip, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr)); err == nil {
		return ip
	}
	return ""
}

// GetIPAddress 通过IP获取地址
func GetIPAddress(ip string) (province string, city string, err error) {
	client := &http.Client{Timeout: 5 * time.Second}
	// 优先使用 ip138（如未配置 token 或请求失败则自动降级）。
	if p, c, ip138Err := getIPAddressByIP138(client, ip); ip138Err == nil {
		return p, c, nil
	}
	var resp *http.Response
	resp, err = client.Get(fmt.Sprintf("https://restapi.amap.com/v3/ip?key=7e30415c3e9ce73d93d20189b9539be8&ip=%s", ip))
	if err != nil {
		return getIPAddressFallback(client, ip)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return getIPAddressFallback(client, ip)
	}
	var data []byte
	data, err = io.ReadAll(resp.Body)
	if err != nil {
		return getIPAddressFallback(client, ip)
	}
	var resultMap map[string]interface{}
	resultMap, err = JsonToMap(string(data))
	if err != nil {
		return getIPAddressFallback(client, ip)
	}
	// 高德返回 status=0 时代表请求不合法/鉴权失败等，改走兜底。
	if status, _ := resultMap["status"].(string); status != "1" {
		return getIPAddressFallback(client, ip)
	}
	provinceObj := resultMap["province"]
	cityObj := resultMap["city"]
	if provinceObj != nil && cityObj != nil {
		p, ok1 := provinceObj.(string)
		if !ok1 {
			return
		}
		c, ok2 := cityObj.(string)
		if !ok2 {
			return
		}
		province, city = p, c
		return
	}
	return getIPAddressFallback(client, ip)
}

func getIPAddressByIP138(client *http.Client, ip string) (province string, city string, err error) {
	token := strings.TrimSpace(os.Getenv("IP138_TOKEN"))
	if token == "" {
		return "", "", errors.New("ip138 token is empty")
	}
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://api.ip138.com/ip/?ip=%s&datatype=json", ip), nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("token", token)
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", errors.New("ip138 request failed")
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}
	resultMap, err := JsonToMap(string(data))
	if err != nil {
		return "", "", err
	}
	ret, _ := resultMap["ret"].(string)
	if ret != "ok" {
		return "", "", errors.New("ip138 response invalid")
	}
	dataArr, ok := resultMap["data"].([]interface{})
	if !ok || len(dataArr) == 0 {
		return "", "", errors.New("ip138 data invalid")
	}
	parts := make([]string, 0, len(dataArr))
	for _, item := range dataArr {
		v, _ := item.(string)
		v = strings.TrimSpace(v)
		if v == "" || v == "中国" {
			continue
		}
		parts = append(parts, v)
	}
	if len(parts) == 0 {
		return "", "", errors.New("ip138 area empty")
	}
	province = parts[0]
	if len(parts) > 1 {
		city = parts[1]
	}
	return province, city, nil
}

func getIPAddressFallback(client *http.Client, ip string) (province string, city string, err error) {
	resp, err := client.Get(fmt.Sprintf("https://ipwho.is/%s?lang=zh", ip))
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", errors.New("查询地址失败！")
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}
	resultMap, err := JsonToMap(string(data))
	if err != nil {
		return "", "", err
	}
	// ipwho.is: {"success":true,...}
	if success, ok := resultMap["success"].(bool); ok && !success {
		return "", "", errors.New("查询地址失败！")
	}
	if p, ok := resultMap["region"].(string); ok {
		province = strings.TrimSpace(p)
	}
	if c, ok := resultMap["city"].(string); ok {
		city = strings.TrimSpace(c)
	}
	if province == "" && city == "" {
		return "", "", errors.New("查询地址失败！")
	}
	return province, city, nil
}

// GetIntranetIP 获取本机IP
func GetIntranetIP() (ips []string, err error) {
	ips = make([]string, 0)

	ifaces, e := net.Interfaces()
	if e != nil {
		return ips, e
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue // interface down
		}

		if iface.Flags&net.FlagLoopback != 0 {
			continue // loopback interface
		}

		// ignore docker and warden bridge
		if strings.HasPrefix(iface.Name, "docker") || strings.HasPrefix(iface.Name, "w-") {
			continue
		}

		addrs, e := iface.Addrs()
		if e != nil {
			return ips, e
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			if ip == nil || ip.IsLoopback() {
				continue
			}

			ip = ip.To4()
			if ip == nil {
				continue // not an ipv4 address
			}

			ipStr := ip.String()
			if IsIntranet(ipStr) {
				ips = append(ips, ipStr)
			}
		}
	}

	return ips, nil
}

// IsIntranet IsIntranet
func IsIntranet(ipStr string) bool {
	if strings.HasPrefix(ipStr, "10.") || strings.HasPrefix(ipStr, "192.168.") {
		return true
	}

	if strings.HasPrefix(ipStr, "172.") {
		// 172.16.0.0-172.31.255.255
		arr := strings.Split(ipStr, ".")
		if len(arr) != 4 {
			return false
		}

		second, err := strconv.ParseInt(arr[1], 10, 64)
		if err != nil {
			return false
		}

		if second >= 16 && second <= 31 {
			return true
		}
	}

	return false
}
