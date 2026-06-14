import { describe, expect, it } from "vitest";
import { formatBytes, shortHash } from "./format";

describe("formatBytes", () => {
  it("handles zero and negatives", () => {
    expect(formatBytes(0)).toBe("0 B");
    expect(formatBytes(-5)).toBe("0 B");
  });
  it("scales units", () => {
    expect(formatBytes(512)).toBe("512 B");
    expect(formatBytes(1024)).toBe("1.0 KB");
    expect(formatBytes(1024 * 1024)).toBe("1.0 MB");
    expect(formatBytes(1073741824)).toBe("1.0 GB");
  });
});

describe("shortHash", () => {
  it("truncates", () => {
    expect(shortHash("abcdef1234567890", 6)).toBe("abcdef");
    expect(shortHash("")).toBe("");
  });
});
