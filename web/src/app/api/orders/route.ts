import { NextRequest } from "next/server";
import { proxyOrderAPI } from "@/lib/order-api";

export async function GET(req: NextRequest) {
  return proxyOrderAPI(req, "/orders");
}

export async function POST(req: NextRequest) {
  return proxyOrderAPI(req, "/orders", { method: "POST", body: await req.text() });
}
