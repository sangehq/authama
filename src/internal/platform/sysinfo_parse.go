package platform

import (
	"bufio"
	"strings"
)

// The Linux readers, kept free of a build tag so their tests run everywhere.

// osRelease reads the distribution's name and version from os-release(5).
func osRelease(si *SystemInfo, text string) {
	kv := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok {
			kv[k] = strings.Trim(v, `"'`)
		}
	}
	si.OS = kv["NAME"]
	if si.OS == "" {
		si.OS = "Linux"
	}
	si.OSVersion = kv["VERSION"]
	if si.OSVersion == "" {
		si.OSVersion = kv["VERSION_ID"]
	}
	si.OSBuild = kv["BUILD_ID"]
}

// cpuInfo reads the processor name and the physical core count from
// /proc/cpuinfo: "model name" on x86, "Model" or "Hardware" on many ARM boards.
func cpuInfo(si *SystemInfo, text string) {
	cores := map[string]bool{}
	var physical string
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		switch k {
		case "model name", "Model", "Hardware", "cpu model":
			if si.CPU == "" && v != "" {
				si.CPU = v
			}
		case "physical id":
			physical = v
		case "core id":
			cores[physical+"/"+v] = true
		}
	}
	si.Cores = len(cores)
}
