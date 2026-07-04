"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";

// Shape of the Go API's JSON after the sqlc emit_json_tags cleanup:
// lowercase snake_case both ways.
type Order = {
  id: string;
  item: string;
  quantity: number;
  price: number | string; // pgtype.Numeric may serialize as a number or string
  user_id: string;
};

type Draft = { item: string; quantity: string; price: string };

const emptyDraft: Draft = { item: "", quantity: "1", price: "0.00" };

function money(v: number | string): number {
  const n = typeof v === "number" ? v : parseFloat(v);
  return Number.isFinite(n) ? n : 0;
}

export default function OrdersLedger() {
  const [orders, setOrders] = useState<Order[] | null>(null);
  const [draft, setDraft] = useState<Draft>(emptyDraft);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [sessionExpired, setSessionExpired] = useState(false);

  // Note: no synchronous setState before the first await — this function
  // is invoked from an effect, and react-hooks/set-state-in-effect
  // (correctly) forbids synchronous state updates in effect bodies. All
  // updates below happen in the async continuation, which is allowed;
  // clearing the error on success replaces the old clear-upfront reset.
  const load = useCallback(async () => {
    const res = await fetch("/api/orders", { cache: "no-store" }).catch(
      () => null,
    );
    if (!res) {
      setError("Couldn't reach the server. Try again.");
      return;
    }
    if (res.status === 401) {
      setSessionExpired(true);
      return;
    }
    if (!res.ok) {
      setError("Couldn't load orders. Try again.");
      return;
    }
    setError(null);
    setOrders(await res.json());
  }, []);

  useEffect(() => {
    // Known false positive in react-hooks/set-state-in-effect: every
    // setState inside load() runs *after* an await (async continuation,
    // which the rule's own docs allow), but the rule's interprocedural
    // analysis flags any setState-containing useCallback invoked from an
    // effect regardless of await boundaries. Minimal repro of this exact
    // shape: https://github.com/facebook/react/issues/34905 — remove this
    // suppression once that's fixed upstream.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void load();
  }, [load]);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    const body = JSON.stringify({
      item: draft.item.trim(),
      quantity: parseInt(draft.quantity, 10),
      price: parseFloat(draft.price),
    });
    const res = editingId
      ? await fetch(`/api/orders/${encodeURIComponent(editingId)}`, {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body,
        })
      : await fetch("/api/orders", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body,
        });
    setBusy(false);
    if (res.status === 401) {
      setSessionExpired(true);
      return;
    }
    if (!res.ok) {
      const payload = await res.json().catch(() => null);
      setError(
        payload?.error ??
          (editingId ? "Couldn't save changes." : "Couldn't add the order."),
      );
      return;
    }
    setDraft(emptyDraft);
    setEditingId(null);
    await load();
  }

  async function remove(id: string) {
    setError(null);
    const res = await fetch(`/api/orders/${encodeURIComponent(id)}`, {
      method: "DELETE",
    });
    if (res.status === 401) {
      setSessionExpired(true);
      return;
    }
    if (!res.ok && res.status !== 404) {
      setError("Couldn't delete the order.");
      return;
    }
    if (editingId === id) {
      setEditingId(null);
      setDraft(emptyDraft);
    }
    await load();
  }

  function startEdit(o: Order) {
    setEditingId(o.id);
    setDraft({
      item: o.item,
      quantity: String(o.quantity),
      price: money(o.price).toFixed(2),
    });
  }

  function cancelEdit() {
    setEditingId(null);
    setDraft(emptyDraft);
  }

  if (sessionExpired) {
    return (
      <div className="rounded-sm border border-line bg-card p-6">
        <p className="text-sm">
          Your session expired. Sign in again to pick up where you left off.
        </p>
        <Link
          href="/"
          className="mt-4 inline-block rounded bg-teal px-4 py-2 font-[family-name:var(--font-mono)] text-sm text-card hover:bg-teal-deep"
        >
          Back to sign in
        </Link>
      </div>
    );
  }

  const total = (orders ?? []).reduce(
    (sum, o) => sum + o.quantity * money(o.price),
    0,
  );

  return (
    <section className="perforated rounded-sm border border-line bg-card px-5 pb-8 pt-5 shadow-sm sm:px-7">
      <div className="flex items-baseline justify-between">
        <h2 className="font-[family-name:var(--font-mono)] text-[11px] uppercase tracking-[0.14em] text-muted">
          your ledger
        </h2>
        <button
          onClick={() => void load()}
          className="font-[family-name:var(--font-mono)] text-xs text-muted underline-offset-2 hover:text-teal hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-teal"
        >
          Refresh
        </button>
      </div>

      {error && (
        <p role="alert" className="mt-4 text-sm text-danger">
          {error}
        </p>
      )}

      {orders === null ? (
        <p className="mt-6 font-[family-name:var(--font-mono)] text-sm text-muted">
          Loading your orders…
        </p>
      ) : orders.length === 0 ? (
        <p className="mt-6 font-[family-name:var(--font-mono)] text-sm text-muted">
          No orders yet. Add your first below.
        </p>
      ) : (
        <ul className="mt-5 space-y-1 font-[family-name:var(--font-mono)] text-sm">
          {orders.map((o) => (
            <li
              key={o.id}
              className={`row-in group flex items-baseline rounded px-1 py-1.5 ${
                editingId === o.id ? "bg-paper" : ""
              }`}
            >
              <span className="min-w-0 truncate">{o.item}</span>
              <span className="leader" />
              <span className="whitespace-nowrap tabular-nums">
                {o.quantity} × {money(o.price).toFixed(2)}
              </span>
              <span className="ml-4 w-20 text-right tabular-nums text-money">
                {(o.quantity * money(o.price)).toFixed(2)}
              </span>
              <span className="ml-3 flex gap-2 opacity-0 transition-opacity focus-within:opacity-100 group-hover:opacity-100">
                <button
                  onClick={() => startEdit(o)}
                  className="text-xs text-muted hover:text-teal focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-teal"
                  aria-label={`Edit ${o.item}`}
                >
                  Edit
                </button>
                <button
                  onClick={() => void remove(o.id)}
                  className="text-xs text-muted hover:text-danger focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-danger"
                  aria-label={`Delete ${o.item}`}
                >
                  Delete
                </button>
              </span>
            </li>
          ))}
        </ul>
      )}

      {orders !== null && orders.length > 0 && (
        <div className="mt-4 flex items-baseline border-t border-line pt-3 font-[family-name:var(--font-mono)] text-sm font-semibold">
          <span>total</span>
          <span className="leader" />
          <span className="tabular-nums text-money">{total.toFixed(2)}</span>
        </div>
      )}

      <form
        onSubmit={submit}
        className="mt-8 grid grid-cols-2 gap-3 border-t border-dashed border-line pt-6 sm:grid-cols-[1fr_5.5rem_7rem_auto]"
      >
        <label className="col-span-2 sm:col-span-1">
          <span className="mb-1 block font-[family-name:var(--font-mono)] text-[11px] uppercase tracking-[0.12em] text-muted">
            item
          </span>
          <input
            required
            value={draft.item}
            onChange={(e) => setDraft({ ...draft, item: e.target.value })}
            placeholder="Mechanical keyboard"
            className="w-full rounded border border-line bg-paper px-3 py-2 text-sm outline-none focus:border-teal focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-teal"
          />
        </label>
        <label>
          <span className="mb-1 block font-[family-name:var(--font-mono)] text-[11px] uppercase tracking-[0.12em] text-muted">
            qty
          </span>
          <input
            required
            type="number"
            min={1}
            step={1}
            value={draft.quantity}
            onChange={(e) => setDraft({ ...draft, quantity: e.target.value })}
            className="w-full rounded border border-line bg-paper px-3 py-2 font-[family-name:var(--font-mono)] text-sm tabular-nums outline-none focus:border-teal focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-teal"
          />
        </label>
        <label>
          <span className="mb-1 block font-[family-name:var(--font-mono)] text-[11px] uppercase tracking-[0.12em] text-muted">
            price
          </span>
          <input
            required
            type="number"
            min={0}
            step="0.01"
            value={draft.price}
            onChange={(e) => setDraft({ ...draft, price: e.target.value })}
            className="w-full rounded border border-line bg-paper px-3 py-2 font-[family-name:var(--font-mono)] text-sm tabular-nums outline-none focus:border-teal focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-teal"
          />
        </label>
        <div className="col-span-2 flex items-end gap-2 sm:col-span-1">
          <button
            type="submit"
            disabled={busy}
            className="rounded bg-teal px-4 py-2 font-[family-name:var(--font-mono)] text-sm font-medium text-card transition-colors hover:bg-teal-deep disabled:opacity-60 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-teal"
          >
            {editingId ? "Save changes" : "Add order"}
          </button>
          {editingId && (
            <button
              type="button"
              onClick={cancelEdit}
              className="rounded border border-line bg-card px-4 py-2 font-[family-name:var(--font-mono)] text-sm text-ink hover:border-teal hover:text-teal focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-teal"
            >
              Cancel
            </button>
          )}
        </div>
      </form>
    </section>
  );
}
