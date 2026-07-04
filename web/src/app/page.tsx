import { auth, signIn, signOut } from "@/auth";
import OrdersLedger from "@/components/OrdersLedger";

export default async function Home() {
  const session = await auth();

  if (!session?.user) {
    return <Landing />;
  }

  return (
    <main className="mx-auto flex min-h-screen w-full max-w-3xl flex-col px-5 py-8">
      <header className="mb-8 flex flex-wrap items-baseline gap-x-4 gap-y-2">
        <span className="font-[family-name:var(--font-display)] text-2xl font-semibold tracking-tight">
          orders<span className="text-teal">.</span>
        </span>
        <span className="ml-auto font-[family-name:var(--font-mono)] text-xs text-muted">
          {session.user.email ?? session.user.name ?? "signed in"}
        </span>
        <form
          action={async () => {
            "use server";
            await signOut({ redirectTo: "/" });
          }}
        >
          <button
            type="submit"
            className="rounded border border-line bg-card px-3 py-1.5 font-[family-name:var(--font-mono)] text-xs text-ink transition-colors hover:border-teal hover:text-teal focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-teal"
          >
            Sign out
          </button>
        </form>
      </header>

      <OrdersLedger />

      <footer className="mt-10 font-[family-name:var(--font-mono)] text-[11px] text-muted">
        Session and tokens stay on the server. Signing out clears this
        app&apos;s session only.
      </footer>
    </main>
  );
}

function Landing() {
  return (
    <main className="mx-auto grid min-h-screen w-full max-w-5xl items-center gap-12 px-6 py-16 md:grid-cols-[3fr_2fr]">
      <section>
        <h1 className="font-[family-name:var(--font-display)] text-6xl font-semibold leading-none tracking-tight sm:text-7xl">
          orders<span className="text-teal">.</span>
        </h1>
        <p className="mt-5 max-w-md text-lg text-muted">
          Every order you create, in one ledger. Sign in to see yours —
          nobody else&apos;s, ever.
        </p>
        <div className="mt-8 flex flex-wrap gap-3">
          <form
            action={async () => {
              "use server";
              await signIn("zitadel", { redirectTo: "/" });
            }}
          >
            <button
              type="submit"
              className="rounded bg-teal px-5 py-2.5 font-[family-name:var(--font-mono)] text-sm font-medium text-card transition-colors hover:bg-teal-deep focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-teal"
            >
              Sign in
            </button>
          </form>
          <form
            action={async () => {
              "use server";
              // prompt=create sends the user straight to Zitadel's hosted
              // registration page — registration by configuration, no
              // custom forms (HANDOFF goal 2).
              await signIn("zitadel", { redirectTo: "/" }, { prompt: "create" });
            }}
          >
            <button
              type="submit"
              className="rounded border border-line bg-card px-5 py-2.5 font-[family-name:var(--font-mono)] text-sm text-ink transition-colors hover:border-teal hover:text-teal focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-teal"
            >
              Create account
            </button>
          </form>
        </div>
        <p className="mt-4 font-[family-name:var(--font-mono)] text-xs text-muted">
          Sign-in happens on our identity service. This site never sees
          your password.
        </p>
      </section>

      {/* Decorative sample receipt — the ledger the app is about. */}
      <aside aria-hidden="true" className="hidden md:block">
        <div className="perforated rotate-1 rounded-sm border border-line bg-card px-6 pb-6 pt-5 shadow-sm">
          <p className="font-[family-name:var(--font-mono)] text-[11px] uppercase tracking-[0.14em] text-muted">
            your ledger
          </p>
          <ul className="mt-4 space-y-3 font-[family-name:var(--font-mono)] text-sm">
            {[
              ["Wireless keyboard", "1 × 89.99"],
              ["Dock station", "1 × 149.50"],
              ["Laptop stand", "2 × 42.00"],
            ].map(([item, amount]) => (
              <li key={item} className="flex items-baseline">
                <span>{item}</span>
                <span className="leader" />
                <span className="tabular-nums">{amount}</span>
              </li>
            ))}
          </ul>
          <div className="mt-5 flex items-baseline border-t border-line pt-3 font-[family-name:var(--font-mono)] text-sm font-semibold">
            <span>total</span>
            <span className="leader" />
            <span className="tabular-nums text-money">323.49</span>
          </div>
        </div>
      </aside>
    </main>
  );
}
