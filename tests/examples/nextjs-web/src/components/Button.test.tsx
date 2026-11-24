import { describe, expect, it } from "vitest";
import { renderToString } from "react-dom/server";
import { Button } from "./Button";

describe("Button", () => {
  it("renders a link when href is set", () => {
    expect(renderToString(<Button href="/x">Go</Button>)).toContain("<a");
  });
});
