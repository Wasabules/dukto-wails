import {
  onElementRejected,
  onFileDrop,
  onHistoryAppend,
  onPeerFound,
  onPeerGone,
  onPendingSession,
  onReceiveDone,
  onReceiveError,
  onReceiveProgress,
  onReceiveStart,
  onReceivingChanged,
  onSendDone,
  onSendError,
  onSendProgress,
  onSendStart,
} from './dukto';
import { pendingSession } from './stores/pending';
import { removePeer, upsertPeer } from './stores/peers';
import { pushHistory } from './stores/received';
import {
  receiveProgress,
  sendByPeer,
  sendProgress,
} from './stores/progress';
import { receiving } from './stores/receiving';
import { showToast } from './stores/toast';

export interface DropHandler {
  (detail: { paths: string[]; x: number; y: number }): void;
}

// wireEvents subscribes to every Wails event that feeds the UI stores and
// returns a single teardown that unsubscribes them all. `onDrop` stays a
// callback because the hit-test depends on the live DOM, which belongs in
// the component, not a store.
export function wireEvents(onDrop: DropHandler): () => void {
  const unsubs: Array<() => void> = [
    onPeerFound((p) => upsertPeer(p)),
    onPeerGone((p) => removePeer(p)),
    onReceiveStart((p) => {
      const t = Date.now();
      receiveProgress.set({
        bytes: 0,
        total: p.bytes,
        label: 'Receiving…',
        startedAt: t,
        startedBytes: 0,
      });
      showToast(`Receiving ${p.total} element${p.total === 1 ? '' : 's'}…`);
    }),
    onHistoryAppend((h) => pushHistory(h, { prepend: true })),
    onReceiveProgress((p) => {
      receiveProgress.update((cur) =>
        cur ? { ...cur, bytes: p.bytes, total: p.total } : cur,
      );
    }),
    onReceiveDone(() => {
      receiveProgress.set(null);
      showToast('Transfer complete.');
    }),
    onReceiveError((m) => {
      receiveProgress.set(null);
      showToast(`Receive error: ${m}`);
    }),
    onSendStart((p) => {
      const t = Date.now();
      sendProgress.set({
        bytes: 0,
        total: p.total,
        label: `Sending to ${p.peer}…`,
        startedAt: t,
        startedBytes: 0,
      });
      sendByPeer.update((m) => {
        m.set(p.peer, { bytes: 0, total: p.total });
        return new Map(m);
      });
    }),
    onSendProgress((p) => {
      sendProgress.update((cur) =>
        cur ? { ...cur, bytes: p.bytes, total: p.total } : cur,
      );
      // Backend emits bytes without a peer tag; mirror into every active
      // entry. With one live transfer at a time this is "the active peer".
      sendByPeer.update((m) => {
        for (const [k, v] of m) {
          m.set(k, { bytes: p.bytes, total: p.total || v.total });
        }
        return new Map(m);
      });
    }),
    onSendDone((p) => {
      sendProgress.set(null);
      sendByPeer.update((m) => {
        m.delete(p.peer);
        return new Map(m);
      });
    }),
    onSendError((m) => {
      sendProgress.set(null);
      sendByPeer.set(new Map());
      showToast(`Send error: ${m}`);
    }),
    onReceivingChanged((p) => {
      receiving.set(p.enabled);
      // Only the idle-timer path merits a toast — a user-initiated toggle
      // already produced one at the call site.
      if (p.reason === 'idle' && !p.enabled) {
        showToast('Receiving auto-disabled after inactivity.');
      }
    }),
    onFileDrop(onDrop),
    onElementRejected((p) => {
      showToast(`Rejected ${p.name}: ${p.reason}`);
    }),
    onPendingSession((p) => {
      pendingSession.set(p);
    }),
  ];
  return () => unsubs.forEach((fn) => fn());
}
