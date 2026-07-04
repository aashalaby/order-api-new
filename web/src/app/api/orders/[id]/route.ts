import { NextRequest } from "next/server";
import { proxyOrderAPI } from "@/lib/order-api";

type Ctx = { params: Promise<{ id: string }> };

export async function GET(req: NextRequest, ctx: Ctx) {
  const { id } = await ctx.params;
  return proxyOrderAPI(req, `/orders/${encodeURIComponent(id)}`);
}

export async function PUT(req: NextRequest, ctx: Ctx) {
  const { id } = await ctx.params;
  return proxyOrderAPI(req, `/orders/${encodeURIComponent(id)}`, {
    method: "PUT",
    body: await req.text(),
  });
}

export async function DELETE(req: NextRequest, ctx: Ctx) {
  const { id } = await ctx.params;
  return proxyOrderAPI(req, `/orders/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
}
