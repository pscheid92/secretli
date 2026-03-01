import { validateEmail, validatePassword, validateRequired } from "../validation";

describe("validateEmail", () => {
  it("returns error for empty string", () => {
    expect(validateEmail("")).toBe("Email is required.");
    expect(validateEmail("   ")).toBe("Email is required.");
  });

  it("returns error for invalid email", () => {
    expect(validateEmail("notanemail")).toBe("Please enter a valid email address.");
    expect(validateEmail("missing@tld")).toBe("Please enter a valid email address.");
    expect(validateEmail("@nodomain.com")).toBe("Please enter a valid email address.");
  });

  it("returns null for valid email", () => {
    expect(validateEmail("user@example.com")).toBeNull();
    expect(validateEmail("a+b@sub.domain.org")).toBeNull();
  });
});

describe("validatePassword", () => {
  it("returns error for empty string", () => {
    expect(validatePassword("")).toBe("Password is required.");
  });

  it("returns error for short password", () => {
    expect(validatePassword("1234567")).toBe("Password must be at least 8 characters.");
  });

  it("returns null for valid password", () => {
    expect(validatePassword("12345678")).toBeNull();
    expect(validatePassword("a-very-long-password")).toBeNull();
  });
});

describe("validateRequired", () => {
  it("returns error for empty/whitespace string", () => {
    expect(validateRequired("", "Name")).toBe("Name is required.");
    expect(validateRequired("   ", "Name")).toBe("Name is required.");
  });

  it("returns null for non-empty string", () => {
    expect(validateRequired("hello", "Name")).toBeNull();
  });
});
