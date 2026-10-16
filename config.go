package main

import (
	"encoding/json"
	"fmt"
	"log"
	"slices"
)

func main() {
	client := NewClient(Token, APIEndpoint)

	m, err := client.user()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	fmt.Printf("client: %+v\n", m)
	rep0 := m.Replicants[0].Code

	inventory, err := client.inventory(rep0)
	if err != nil {
		fmt.Println("inventory:", err)
		return
	}

	fmt.Printf("inventory: %+v\n", inventory)

	r, err := client.replicant(rep0)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	fmt.Printf("replicant: %+v\n", r)
	if err := client.deployMiners(r); err != nil {
		fmt.Println("deploying miners:", err)
		return
	}
	fmt.Printf("deployed miners\n")

	locInfo, err := client.locationInfo(r.Location)
	if err != nil {
		fmt.Println("location info:", err)
		return
	}
	fmt.Printf("location info: %s\n", locInfo)
	startSearch(client, locInfo)
	if err := assignMiners(client, locInfo); err != nil {
		fmt.Println("assigning miners:", err)
		return
	}
	fmt.Printf("assigned miners\n")

}

func assignMiners(c *Client, loc *Location) error {
	if len(loc.ResourceSites) == 0 {
		return fmt.Errorf("no resource sites found")
	}
	site := loc.ResourceSites[0]
	var resources []string
	for k, v := range site.Remaining {
		if v > 0 {
			resources = append(resources, k)
		}
	}
	if len(resources) == 0 {
		return fmt.Errorf("no resources remaining")
	}
	slices.SortFunc(resources, func(a, b string) int {
		return site.Remaining[b] - site.Remaining[a]
	})
	for _, r := range resources {
		fmt.Printf("resource: %s, remaining: %d\n", r, site.Remaining[r])
	}
	pos := 0
	for _, d := range loc.Devices {
		if !(d.DeviceType == miningDrone && d.Status == "idle") {
			continue
		}
		resource := resources[pos]
		pos = (pos + 1) % len(resources)
		startMining(c, d.DeviceCode, resource)

	}
	return nil
}

func startMining(c *Client, id, resource string) error {
	cmd := &Command{
		Command:      "start_mining",
		ResourceType: resource,
	}
	if body, err := c.postJSON(cmd, "/devices/%s", id); err != nil {
		return fmt.Errorf("assigning %s to %s: %v, %s\n", resource, id, err, body)
	} else {
		var m MiningResponse
		if err := json.Unmarshal(body, &m); err != nil {
			return fmt.Errorf("unmarshalling mining response: %v", err)
		}
		log.Printf("assigned %s to %s: %+v\n", resource, id, m)
	}
	return nil
}

func startSearch(c *Client, locInfo *Location) {
	for _, d := range locInfo.Devices {
		if d.DeviceType != surveyDrone {
			continue
		}
		fmt.Printf("survey drone: %+v\n", d)
		if response, err := c.search(d.DeviceCode); err != nil {
			fmt.Println("searching:", err)
		} else {
			fmt.Printf("search response: %+v\n", response)
		}
	}
}
