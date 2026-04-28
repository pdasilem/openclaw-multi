package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"syscall"

	"github.com/pdasilem/openclaw-multi/internal/state"
)

type PeerCred struct {
	UID int
	GID int
	PID int
}

type peerKey struct{}

type PeerCredentialsProvider interface {
	PeerCredentials(*http.Request) (PeerCred, error)
}

type StaticPeerProvider struct {
	Cred PeerCred
	Err  error
}

func (p StaticPeerProvider) PeerCredentials(*http.Request) (PeerCred, error) {
	if p.Err != nil {
		return PeerCred{}, p.Err
	}
	return p.Cred, nil
}

type UnixPeerCredentialsProvider struct{}

func ContextWithPeer(ctx context.Context, c net.Conn) context.Context {
	if uc, ok := c.(*net.UnixConn); ok {
		raw, err := uc.SyscallConn()
		if err == nil {
			var cred *syscall.Ucred
			_ = raw.Control(func(fd uintptr) {
				cred, _ = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
			})
			if cred != nil {
				return context.WithValue(ctx, peerKey{}, PeerCred{UID: int(cred.Uid), GID: int(cred.Gid), PID: int(cred.Pid)})
			}
		}
	}
	return ctx
}

func (UnixPeerCredentialsProvider) PeerCredentials(r *http.Request) (PeerCred, error) {
	cred, ok := r.Context().Value(peerKey{}).(PeerCred)
	if !ok {
		return PeerCred{}, errors.New("missing peer credentials")
	}
	return cred, nil
}

func authorizeUser(ctx context.Context, store *state.Store, username string, cred PeerCred) error {
	if cred.UID == 0 {
		return nil
	}
	admin, err := store.GetAdmin(ctx)
	if err == nil && admin.UID == cred.UID {
		return nil
	}
	user, err := store.GetUser(ctx, username)
	if err != nil {
		return err
	}
	if user.UID != cred.UID {
		return errForbidden
	}
	return nil
}
