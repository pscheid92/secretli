import { formatProtection } from "../format";

describe("formatProtection", () => {
  it("lists both protections when a share has both", () => {
    expect(formatProtection(true, true)).toBe("Password, burn after reading");
  });

  it("names a single protection", () => {
    expect(formatProtection(true, false)).toBe("Password");
    expect(formatProtection(false, true)).toBe("Burn after reading");
  });

  it("calls an unprotected share standard", () => {
    expect(formatProtection(false, false)).toBe("Standard");
  });
});
