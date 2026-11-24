import { Button } from "@/components/Button";

export default function CheckoutPage() {
  return (
    <main>
      <h1>Checkout</h1>
      <form action="/api/checkout" method="post">
        <Button type="submit">Pay now</Button>
      </form>
    </main>
  );
}
