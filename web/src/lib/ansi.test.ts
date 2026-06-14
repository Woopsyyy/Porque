import { describe, expect, it } from "vitest";
import { stripAnsi, toLines } from "./ansi";

describe("stripAnsi", () => {
  it("removes SGR color codes", () => {
    expect(stripAnsi("\x1b[32m INFO\x1b[0m hello")).toBe(" INFO hello");
  });
  it("leaves plain text untouched", () => {
    expect(stripAnsi("Done (16.2s)!")).toBe("Done (16.2s)!");
  });
});

describe("toLines", () => {
  it("strips ansi, splits, and drops empty lines", () => {
    expect(toLines("\x1b[33mWARN\x1b[0m a\r\n\nB\n")).toEqual(["WARN a", "B"]);
  });
});
