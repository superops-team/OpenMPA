package usage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

const snapshotTTL = 5 * time.Minute
const snapshotLimit = 32

type retainedSnapshot struct {
	state   MainProfileState
	page    *mygo.Page
	ctx     context.Context
	scope   string
	expires time.Time
}

// Bridge owns the IPC surface; embedding Service would expose invalid bindings.
type Bridge struct {
	core      *Service
	mu        sync.Mutex
	snapshots map[string]retainedSnapshot
	page      func(context.Context) *mygo.Page
	now       func() time.Time
}

func NewBridge(core *Service) *Bridge {
	return &Bridge{core: core, snapshots: make(map[string]retainedSnapshot), page: mygo.CallerPage, now: time.Now}
}

func (b *Bridge) pruneLocked() {
	state := b.core.conn.State()
	for token, entry := range b.snapshots {
		if entry.ctx.Err() != nil || !b.now().Before(entry.expires) ||
			entry.state.ConnectionEpoch != state.ConnectionEpoch || entry.scope != state.Scope || !state.Connected {
			delete(b.snapshots, token)
		}
	}
}

func (b *Bridge) retain(ctx context.Context, state MainProfileState) (string, *dto.SafeError) {
	page := b.page(ctx)
	if page == nil || ctx.Err() != nil || state.snapshot == nil {
		return "", resourceInputError()
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", dto.Failure("unavailable", "desktop", "never", 0)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pruneLocked()
	if ctx.Err() != nil || !b.core.conn.Current(state.ConnectionEpoch) || state.snapshot.scope != b.core.conn.State().Scope {
		return "", dto.Failure("staleConnection", "transport", "reload", 0)
	}
	if len(b.snapshots) >= snapshotLimit {
		var oldest string
		var expiry time.Time
		for token, entry := range b.snapshots {
			if oldest == "" || entry.expires.Before(expiry) {
				oldest, expiry = token, entry.expires
			}
		}
		delete(b.snapshots, oldest)
	}
	token := hex.EncodeToString(key)
	b.snapshots[token] = retainedSnapshot{state: state, page: page, ctx: ctx,
		scope: state.snapshot.scope, expires: b.now().Add(snapshotTTL)}
	return token, nil
}

func (b *Bridge) claim(ctx context.Context, epoch uint64, token string) (MainProfileState, *dto.SafeError) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pruneLocked()
	entry, found := b.snapshots[token]
	if !found {
		return MainProfileState{}, dto.Failure("conflict", "desktop", "reload", 0)
	}
	if b.page(ctx) == nil || b.page(ctx) != entry.page || ctx != entry.ctx {
		return MainProfileState{}, resourceInputError()
	}
	if epoch != entry.state.ConnectionEpoch || ctx.Err() != nil || !b.core.conn.Current(epoch) {
		return MainProfileState{}, dto.Failure("staleConnection", "transport", "reload", 0)
	}
	delete(b.snapshots, token)
	return entry.state, nil
}
