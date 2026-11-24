import { NextResponse } from "next/server";
import { stripe } from "@/lib/stripe";

export async function POST() {
  const session = await stripe.checkout.sessions.create({
    mode: "payment",
    line_items: [{ price: "price_example", quantity: 1 }],
    success_url: "https://example.com/thanks",
  });
  return NextResponse.redirect(session.url!, 303);
}
