import { NextResponse } from "next/server";
import { stripe } from "@/lib/stripe";
import { db } from "@/lib/db";

export async function POST(request: Request) {
  const signature = request.headers.get("stripe-signature") ?? "";
  const event = stripe.webhooks.constructEvent(
    await request.text(),
    signature,
    process.env.STRIPE_WEBHOOK_SECRET!,
  );
  if (event.type === "checkout.session.completed") {
    await db.order.update({ where: { sessionId: event.data.object.id }, data: { paid: true } });
  }
  return NextResponse.json({ received: true });
}
