package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"

	. "fogflow/common/config"
	. "fogflow/common/ngsi"
)

type SiteInfo struct {
	ExternalAddress string `json:"externalAddress"`
	GeohashID       string `json:"geohashID"`
}

type TreeNode struct {
	MyInfo SiteInfo `json:"siteinfo"`

	Parent    string   `json:"parent"`
	Children  []string `json:"children"`
	Neighbors []string `json:"neighbors"`
}

type SiteNode struct {
	Parent    *SiteNode
	Children  []*SiteNode
	Neighbors []*SiteNode

	MyInfo SiteInfo
}

type BroadCastMsg struct {
	MsgType string      `json:"type"`
	From    string      `json:"from"`
	PayLoad interface{} `json:"payload"`
}

type RecvBroadcastMsg struct {
	MsgType string          `json:"type"`
	From    string          `json:"from"`
	PayLoad json.RawMessage `json:"payload"`
}

// if geohashA is the child of geohashB, return true; otherwise, return false
func isSubCell(geohashA string, geohashB string) bool {
	if len(geohashA) > len(geohashB) && strings.Contains(geohashA, geohashB) == true {
		return true
	} else {
		return false
	}
}

// if geohashA is the parent of geohashB, return true; otherwise, return false
func isParentCell(geohashA string, geohashB string) bool {
	if len(geohashA) < len(geohashB) && strings.Contains(geohashB, geohashA) == true {
		return true
	} else {
		return false
	}
}

type Routing struct {
	//current site
	MySiteNode *SiteNode

	//my direct neighbors
	Neighbors []*SiteNode

	//geoscope-based routing table
	GeoRoutingTable map[string]*SiteNode

	//root site
	RootSite *SiteNode

	//lock to synchronize update/read of this routing table
	lock sync.RWMutex
}

func (r *Routing) Init(rootDiscovery string, mySite SiteInfo) {
	r.GeoRoutingTable = make(map[string]*SiteNode)

	INFO.Println(rootDiscovery, mySite.ExternalAddress)

	if mySite.ExternalAddress == rootDiscovery {
		sNode := SiteNode{}
		sNode.Parent = nil
		sNode.Children = make([]*SiteNode, 0)
		sNode.Neighbors = make([]*SiteNode, 0)
		sNode.MyInfo = mySite

		r.MySiteNode = &sNode
		r.RootSite = &sNode

		r.GeoRoutingTable[mySite.GeohashID] = &sNode
	} else {
		// fetch the routing table from the cloud site
		r.fetchRoutingTable(rootDiscovery)

		INFO.Println("fetched the global routing table")

		sNode := SiteNode{}
		sNode.MyInfo.ExternalAddress = mySite.ExternalAddress
		sNode.MyInfo.GeohashID = mySite.GeohashID
		sNode.Children = make([]*SiteNode, 0)
		sNode.Neighbors = make([]*SiteNode, 0)
		sNode.Parent = nil

		r.MySiteNode = &sNode

		r.updateWithNewSite(&sNode)

		INFO.Println("updated my routing table")

		msg := BroadCastMsg{}
		msg.MsgType = "GEOSCOPE_ANNOUNCEMENT"
		msg.From = mySite.GeohashID
		msg.PayLoad = mySite

		r.Broadcast(&msg, mySite.GeohashID)
	}
}

func (r *Routing) fetchRoutingTable(rootSiteIP string) error {
	resp, err := http.Get("http://" + rootSiteIP + "/ngsi9/sitelist")
	if err != nil {
		fmt.Println(err)
		return err
	}
	defer resp.Body.Close()

	text, _ := ioutil.ReadAll(resp.Body)

	var siteList []TreeNode

	if err := json.Unmarshal(text, &siteList); err != nil {
		return err
	}

	INFO.Println(siteList)

	r.Deserialization(siteList)

	return nil
}

// go through the tree from the top, find out the leaf node that covers the point
func (r *Routing) GetSite(location Point) SiteInfo {
	site := SiteInfo{}

	return site
}

func (r *Routing) GetMiniCover(geohashID string) []SiteInfo {
	involvedSites := make([]SiteInfo, 0)

	return involvedSites
}

func (r *Routing) GetNeighbors() []SiteInfo {
	neighboringSites := make([]SiteInfo, 0)

	for _, neighbor := range r.Neighbors {
		site := (*neighbor).MyInfo
		neighboringSites = append(neighboringSites, site)
	}

	return neighboringSites
}

func (r *Routing) Broadcast(msg *BroadCastMsg, from string) {
	if r.MySiteNode.Parent != nil {
		address := r.MySiteNode.Parent.MyInfo.ExternalAddress
		geohashID := r.MySiteNode.Parent.MyInfo.GeohashID
		if geohashID != from {
			r.SendMessage(address, msg)
		}
	}

	for _, child := range r.MySiteNode.Children {
		address := child.MyInfo.ExternalAddress
		geohashID := child.MyInfo.GeohashID
		if geohashID != from {
			r.SendMessage(address, msg)
		}
	}

}

func (r *Routing) ReceiveBroadcast(msg *RecvBroadcastMsg) {
	if msg.MsgType == "GEOSCOPE_ANNOUNCEMENT" {
		newSite := SiteInfo{}

		if err := json.Unmarshal(msg.PayLoad, &newSite); err != nil {
			ERROR.Println("received an error broadcast message")
			return
		}

		newMsg := BroadCastMsg{}
		newMsg.MsgType = msg.MsgType
		newMsg.From = r.MySiteNode.MyInfo.GeohashID
		newMsg.PayLoad = newSite

		from := msg.From
		go r.Broadcast(&newMsg, from)

		siteNode := SiteNode{}
		siteNode.MyInfo.ExternalAddress = newSite.ExternalAddress
		siteNode.MyInfo.GeohashID = newSite.GeohashID
		siteNode.Children = make([]*SiteNode, 0)
		siteNode.Neighbors = make([]*SiteNode, 0)
		siteNode.Parent = nil

		r.updateWithNewSite(&siteNode)
	}
}

func (r *Routing) SendMessage(address string, msg *BroadCastMsg) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", "http://"+address+"/ngsi9/broadcast", bytes.NewBuffer(body))
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		ERROR.Println(err)
		return err
	}
	defer resp.Body.Close()

	text, err := ioutil.ReadAll(resp.Body)
	INFO.Println(string(text))
	return err
}

func (r *Routing) updateWithNewSite(siteNode *SiteNode) {
	newSite := siteNode.MyInfo

	if geohashID := newSite.GeohashID; r.GeoRoutingTable[geohashID] != nil {
		// already exist in the table
		return
	}

	// add it into the hashtab
	r.GeoRoutingTable[newSite.GeohashID] = siteNode

	// find out the location of this new site in the global tree

	found := false

	temp := r.RootSite
	for len(temp.Children) != 0 && found == false {
		// if the new site belong to one of the existing child node
		continueSearch := false
		for _, child := range temp.Children {
			if isSubCell(newSite.GeohashID, child.MyInfo.GeohashID) == true {
				temp = child
				continueSearch = true
				break
			}

			if isParentCell(newSite.GeohashID, child.MyInfo.GeohashID) == true {
				siteNode.Parent = temp
				siteNode.Children = append(siteNode.Children, child)

				child.Parent = siteNode
				found = true
				break
			}
		}

		if continueSearch == false {
			break
		}
	}

	// reach to a leaf node and the new site is its subcell
	if isSubCell(newSite.GeohashID, temp.MyInfo.GeohashID) == true && found == false {
		siteNode.Parent = temp
		temp.Children = append(temp.Children, siteNode)
	}
}

func (r *Routing) Serialization() []TreeNode {
	siteList := make([]TreeNode, 0)

	for _, v := range r.GeoRoutingTable {
		treeNode := TreeNode{}

		treeNode.MyInfo = v.MyInfo

		if v.Parent == nil {
			treeNode.Parent = "nil"
		} else {
			treeNode.Parent = v.Parent.MyInfo.GeohashID
		}

		treeNode.Children = make([]string, 0)
		for _, node := range v.Children {
			treeNode.Children = append(treeNode.Children, node.MyInfo.GeohashID)
		}

		treeNode.Neighbors = make([]string, 0)
		for _, node := range v.Neighbors {
			treeNode.Neighbors = append(treeNode.Neighbors, node.MyInfo.GeohashID)
		}

		siteList = append(siteList, treeNode)
	}

	return siteList
}

func (r *Routing) Deserialization(siteList []TreeNode) {
	r.lock.Lock()
	defer r.lock.Unlock()

	// build the hashmap
	for _, node := range siteList {
		siteNode := SiteNode{}
		siteNode.MyInfo = node.MyInfo
		r.GeoRoutingTable[node.MyInfo.GeohashID] = &siteNode
	}

	INFO.Println("build the hashmap")

	// link them to construct the tree
	for _, node := range siteList {
		geohashID := node.MyInfo.GeohashID

		siteNode := r.GeoRoutingTable[geohashID]

		if node.Parent == "nil" {
			siteNode.Parent = nil
		} else {
			siteNode.Parent = r.GeoRoutingTable[node.Parent]
		}

		siteNode.Children = make([]*SiteNode, 0)
		for _, geohash := range node.Children {
			siteNode.Children = append(siteNode.Children, r.GeoRoutingTable[geohash])
		}

		siteNode.Neighbors = make([]*SiteNode, 0)
		for _, geohash := range node.Neighbors {
			siteNode.Neighbors = append(siteNode.Neighbors, r.GeoRoutingTable[geohash])
		}
	}

	INFO.Println("link them to construct the tree")

	// identify the root site
	for _, v := range r.GeoRoutingTable {
		if v.Parent == nil {
			r.RootSite = v
			break
		}

		parent := v.Parent
		for parent.Parent != nil {
			parent = parent.Parent
		}

		r.RootSite = parent
		break
	}

	INFO.Println("identify the root site")

}
