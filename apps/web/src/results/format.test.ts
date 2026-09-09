import { describe, expect, it } from "vitest";
import { ZERO_TOLERANCE, formatNumber, isDisplayZero } from "./format";

describe("format", () => {
  it("renders entries at or below the tolerance as 0, using the tolerance it is given", () => {
    expect(isDisplayZero(2.7e-17)).toBe(true);
    expect(isDisplayZero(2.7e-17, ZERO_TOLERANCE)).toBe(true);
    expect(isDisplayZero(1e-9, 1e-8)).toBe(true);
    expect(isDisplayZero(1e-9)).toBe(false);
    expect(formatNumber(1e-9, false, 1e-8)).toBe("0");
    expect(formatNumber(1e-9)).toBe("1.000000e-9");
  });

  it("never rewrites raw values", () => {
    expect(formatNumber(2.7e-17, true, 1)).toBe("2.7e-17");
  });
});
