package db

import (
	"errors"
	"log"
	"strconv"
	"sync"
	"time"

	"gitlab.com/raspberry.tech/wireguard-manager-and-api/src/logger"
	"gitlab.com/raspberry.tech/wireguard-manager-and-api/src/manager"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	"gorm.io/gorm"
)

// peerMu serialises everything that adds or removes peers on the interface or writes
// BandwidthUsed, so bwAccounted always matches the kernel counters it describes.
var peerMu sync.Mutex

// bwAccounted holds, per public key, how many bytes of the peer's current kernel
// TransmitBytes counter are already included in Subscription.BandwidthUsed. The kernel
// counter restarts at zero whenever a peer is removed and re-added.
var bwAccounted = map[string]int64{}

func AddRemovePeers() bool {
	peerMu.Lock()
	defer peerMu.Unlock()

	getInterfaces, err := manager.GetInterfaces()
	if !logger.ErrorHandler("Info - Finding interfaces", err) {
		return false
	}

	for interfaces := 0; interfaces < len(getInterfaces); interfaces++ { //get interfaces
		for peer := 0; peer < len(getInterfaces[interfaces].Peers); peer++ {
			currentPeer := getInterfaces[interfaces].Peers[peer] //get the current peer in for loop
			interfaceName := getInterfaces[interfaces].Name
			if manager.AddRemovePeer(currentPeer, interfaceName) {
				flushPeerBW(currentPeer)
				forgetPeerBW(currentPeer.PublicKey.String())
			}
		}
	}
	return true
}

func BWPeerCheck() bool {
	peerMu.Lock()
	defer peerMu.Unlock()

	getInterfaces, err := manager.GetInterfaces()
	if !logger.ErrorHandler("Info - Finding interfaces", err) {
		return false
	}

	db := DBSystem
	currentTime := time.Now().UTC()
	for interfaces := 0; interfaces < len(getInterfaces); interfaces++ { //get interfaces
		for peer := 0; peer < len(getInterfaces[interfaces].Peers); peer++ { //get each peer in the wg interface
			currentPeer := getInterfaces[interfaces].Peers[peer] //get the current peer in for loop
			pubKeyStr := currentPeer.PublicKey.String()

			flushPeerBW(currentPeer)

			var subStruct Subscription
			resultIP := db.Where("public_key = ?", pubKeyStr).First(&subStruct) //find subscription record
			if errors.Is(resultIP.Error, gorm.ErrRecordNotFound) {
				log.Println("Could not find public key in database: ", pubKeyStr)
				continue
			}

			bwLimit := subStruct.BandwidthAllotted
			subFormatted, subErr := time.Parse("2006-Jan-02 03:04:05 PM", subStruct.SubscriptionEnd)
			if !logger.ErrorHandler("Error - Parsing stored time ", subErr) {
				continue
			}
			if (subStruct.BandwidthUsed > bwLimit || currentTime.After(subFormatted)) && bwLimit != 0 {
				keyID := subStruct.KeyID
				disableKey(strconv.Itoa(keyID)) //disable key if bandwidth limit reached or subscription end
				log.Println("Info - Disabling key, bw or sub has ended, KeyID: ", keyID)
			}
		}
	}
	return true
}

// flushPeerBW adds the whole megabytes the peer transmitted since the last flush to
// BandwidthUsed. Callers must hold peerMu.
func flushPeerBW(currentPeer wgtypes.Peer) {
	pubKey := currentPeer.PublicKey.String()
	accounted := bwAccounted[pubKey]
	if currentPeer.TransmitBytes < accounted { // peer was re-added, counter restarted
		accounted = 0
	}

	usedMB := (currentPeer.TransmitBytes - accounted) / 1000000
	if usedMB > 0 {
		result := DBSystem.Model(&Subscription{}).
			Where("public_key = ?", pubKey).
			Update("bandwidth_used", gorm.Expr("bandwidth_used + ?", usedMB))
		if result.Error != nil {
			log.Println("Error - Updating bandwidth used", result.Error)
			return
		}
		if result.RowsAffected == 0 {
			log.Println("Error - Subscription not found")
			return
		}
		accounted += usedMB * 1000000
	}
	bwAccounted[pubKey] = accounted
}

// forgetPeerBW drops the accounting state of a peer that was removed from the
// interface. Callers must hold peerMu.
func forgetPeerBW(pubKey string) {
	delete(bwAccounted, pubKey)
}
