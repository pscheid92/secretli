import { validateRequired } from "../validation";

describe("validateRequired", () => {
  it("returns error for empty/whitespace string", () => {
    expect(validateRequired("", "Name")).toBe("Name is required.");
    expect(validateRequired("   ", "Name")).toBe("Name is required.");
  });

  it("returns null for non-empty string", () => {
    expect(validateRequired("hello", "Name")).toBeNull();
  });
});
