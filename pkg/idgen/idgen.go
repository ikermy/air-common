// Package idgen generates Snowflake-compatible 64-bit identifiers.
//
// Layout (63 bits, always positive when interpreted as int64):
//
//	41 bits timestamp in milliseconds since 2024-01-01T00:00:00Z
//	10 bits node id (0..1023)
//	12 bits sequence (0..4095)
package idgen

import (
	"fmt"
	"hash/fnv"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/ikermy/air-common/pkg/comdom"
)

const (
	epochMillis    int64 = 1704067200000 // 2024-01-01T00:00:00Z
	nodeBits             = 10
	sequenceBits         = 12
	maxNodeID            = (1 << nodeBits) - 1
	maxSequence          = (1 << sequenceBits) - 1
	nodeShift            = sequenceBits
	timestampShift       = nodeBits + sequenceBits
)

// Generator is safe for concurrent use.
type Generator struct {
	mu       sync.Mutex
	nodeID   uint64
	lastMS   int64
	sequence uint64
	now      func() time.Time
}

// New creates a generator with the given node id.
func New(nodeID uint64) (*Generator, error) {
	if nodeID > maxNodeID {
		return nil, fmt.Errorf("idgen: node id %d exceeds maximum %d", nodeID, maxNodeID)
	}
	return &Generator{
		nodeID: nodeID,
		now:    time.Now,
	}, nil
}

// MustNew is New that panics on invalid node id. Use only during startup.
func MustNew(nodeID uint64) *Generator {
	g, err := New(nodeID)
	if err != nil {
		panic(err)
	}
	return g
}

// NewFromEnv resolves the node id from NODE_ID or SHARD_ID. If neither is set,
// it derives a stable 10-bit id from hostname and pid. Production deployments
// should always set NODE_ID/SHARD_ID explicitly.
func NewFromEnv() (*Generator, error) {
	if raw := firstNonEmpty(os.Getenv("NODE_ID"), os.Getenv("SHARD_ID")); raw != "" {
		nodeID, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("idgen: invalid NODE_ID/SHARD_ID %q: %w", raw, err)
		}
		return New(nodeID)
	}

	host, _ := os.Hostname()
	seed := fmt.Sprintf("%s:%d", host, os.Getpid())
	h := fnv.New32a()
	_, _ = h.Write([]byte(seed))
	return New(uint64(h.Sum32()) & maxNodeID)
}

// Next returns a new identifier.
func (g *Generator) Next() uint64 {
	g.mu.Lock()
	defer g.mu.Unlock()

	nowMS := g.now().UnixMilli()

	// Keep the generator monotonic even if the wall clock moves backwards.
	// We never block; the logical timestamp may run slightly ahead of the real
	// clock until it catches up.
	if nowMS <= g.lastMS {
		g.sequence++
		if g.sequence > maxSequence {
			g.sequence = 0
			nowMS = g.lastMS + 1
		} else {
			nowMS = g.lastMS
		}
	} else {
		g.sequence = 0
	}
	g.lastMS = nowMS

	ts := uint64(nowMS - epochMillis)
	return (ts << timestampShift) | (g.nodeID << nodeShift) | g.sequence
}

func (g *Generator) NextUserID() comdom.UserID           { return comdom.UserID(g.Next()) }
func (g *Generator) NextDialogID() comdom.DialogID       { return comdom.DialogID(g.Next()) }
func (g *Generator) NextResponderID() comdom.ResponderID { return comdom.ResponderID(g.Next()) }
func (g *Generator) NextMessageID() comdom.MessageID     { return comdom.MessageID(g.Next()) }
func (g *Generator) NextBotID() comdom.BotID             { return comdom.BotID(g.Next()) }
func (g *Generator) NextChannelID() comdom.ChannelID     { return comdom.ChannelID(g.Next()) }

// Decode returns the timestamp, node id and sequence encoded in a generated id.
// It is mainly useful for diagnostics and tests.
func Decode(id uint64) (timestamp time.Time, nodeID uint64, sequence uint64) {
	ts := int64(id>>timestampShift) + epochMillis
	nodeID = (id >> nodeShift) & maxNodeID
	sequence = id & maxSequence
	return time.UnixMilli(ts).UTC(), nodeID, sequence
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
