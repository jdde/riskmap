import { describe, expect, it } from "vitest";
import { stripe } from "./stripe";

describe("stripe", () => {
  it("provides the checkout", () => {
    expect(stripe.checkout).toBeDefined();
  });
});
