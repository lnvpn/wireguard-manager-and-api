package db

import (
	"testing"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Subscription{}); err != nil {
		t.Fatal(err)
	}
	DBSystem = db
	bwAccounted = map[string]int64{}
}

func testPeer(t *testing.T) wgtypes.Peer {
	t.Helper()
	key, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	return wgtypes.Peer{PublicKey: key.PublicKey()}
}

func bandwidthUsed(t *testing.T, pubKey string) int64 {
	t.Helper()
	var sub Subscription
	if err := DBSystem.Where("public_key = ?", pubKey).First(&sub).Error; err != nil {
		t.Fatal(err)
	}
	return sub.BandwidthUsed
}

func TestFlushPeerBW(t *testing.T) {
	setupTestDB(t)
	peer := testPeer(t)
	pubKey := peer.PublicKey.String()
	DBSystem.Create(&Subscription{KeyID: 1, PublicKey: pubKey, BandwidthUsed: 10, BandwidthAllotted: 1000})

	steps := []struct {
		name   string
		bytes  int64
		forget bool
		want   int64
	}{
		{"partial megabyte is carried over", 2_500_000, false, 12},
		{"carried bytes count once they make a megabyte", 3_600_000, false, 13},
		{"counter unchanged", 3_600_000, false, 13},
		{"counter restarted after re-add", 1_200_000, false, 14},
		{"forgotten peer starts from zero", 2_100_000, true, 16},
	}
	for _, step := range steps {
		if step.forget {
			forgetPeerBW(pubKey)
		}
		peer.TransmitBytes = step.bytes
		flushPeerBW(peer)
		if got := bandwidthUsed(t, pubKey); got != step.want {
			t.Fatalf("%s: BandwidthUsed = %d, want %d", step.name, got, step.want)
		}
	}
}

func TestFlushPeerBWWithoutSubscription(t *testing.T) {
	setupTestDB(t)
	peer := testPeer(t)
	peer.TransmitBytes = 5_000_000
	flushPeerBW(peer)
	if _, ok := bwAccounted[peer.PublicKey.String()]; ok {
		t.Fatal("accounted bytes recorded for a peer without subscription")
	}
}
