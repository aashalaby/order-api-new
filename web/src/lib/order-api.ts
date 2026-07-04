import { getToken } from "next-auth/jwt";
import { NextRequest, NextResponse } from "next/server";

// BFF proxy to the Go order-api. Same-origin from the browser's point of
// view (the browser only ever talks to /api/orders* on this app), so no
// CORS anywhere in the stack — if something appears to need CORS, the
// architecture took a wrong turn (see HANDOFF constraints).

const ORDER_API_URL = (process.env.ORDER_API_URL ?? "http://localhost:8080").replace(/\/+$/, "");

type ProxyInit = {
  method?: "GET" | "POST" | "PUT" | "DELETE";
  body?: string;
};

export async function proxyOrderAPI(req: NextRequest, path: string, init?: ProxyInit) {
  const secret = process.env.AUTH_SECRET;
  if (!secret) {
    return NextResponse.json(
      { error: "Server misconfigured: AUTH_SECRET is not set" },
      { status: 500 },
    );
  }

  // Decode the encrypted session cookie server-side. The access token
  // never reaches the browser; this is the only place it is read.
  const token = await getToken({
    req,
    secret,
    secureCookie: process.env.NODE_ENV === "production",
  });

  if (!token?.accessToken) {
    return NextResponse.json({ error: "Not signed in" }, { status: 401 });
  }
  if (token.expiresAt && Date.now() >= token.expiresAt * 1000) {
    return NextResponse.json(
      { error: "Session expired — sign in again" },
      { status: 401 },
    );
  }

  const res = await fetch(ORDER_API_URL + path, {
    method: init?.method ?? "GET",
    headers: {
      Authorization: `Bearer ${token.accessToken}`,
      ...(init?.body !== undefined ? { "Content-Type": "application/json" } : {}),
    },
    body: init?.body,
    cache: "no-store",
  });

  // Pass the Go API's status and JSON body through unchanged.
  const text = await res.text();
  return new NextResponse(text.length > 0 ? text : null, {
    status: res.status,
    headers: {
      "Content-Type": res.headers.get("Content-Type") ?? "application/json",
    },
  });
}
