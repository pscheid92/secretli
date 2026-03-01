import { type FormEvent, useState } from "react";
import { Link, useNavigate } from "react-router";
import { toast } from "sonner";
import Spinner from "../components/Spinner";
import { useAuth } from "../context/AuthContext";
import { ApiError } from "../lib/api";
import { validateEmail, validatePassword, validateRequired } from "../lib/validation";

export default function RegisterPage() {
  const { user, register } = useAuth();
  const navigate = useNavigate();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [loading, setLoading] = useState(false);

  if (user) {
    navigate("/", { replace: true });
    return null;
  }

  function validateField(field: string, value: string) {
    let error: string | null = null;
    if (field === "email") error = validateEmail(value);
    if (field === "password") error = validatePassword(value);
    if (field === "displayName") error = validateRequired(value, "Display name");
    setErrors((prev) => {
      const next = { ...prev };
      if (error) next[field] = error;
      else delete next[field];
      return next;
    });
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    const emailErr = validateEmail(email);
    const passErr = validatePassword(password);
    const nameErr = validateRequired(displayName, "Display name");
    if (emailErr || passErr || nameErr) {
      setErrors({
        ...(emailErr ? { email: emailErr } : {}),
        ...(passErr ? { password: passErr } : {}),
        ...(nameErr ? { displayName: nameErr } : {}),
      });
      return;
    }
    setErrors({});
    setLoading(true);
    try {
      await register(email, password, displayName);
      toast.success("Account created");
      navigate("/");
    } catch (err) {
      if (err instanceof ApiError) {
        toast.error(err.message);
      } else {
        toast.error("An unexpected error occurred.");
      }
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="mx-auto max-w-sm space-y-6">
      <div>
        <h1 className="text-2xl font-bold dark:text-white">Create Account</h1>
        <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
          Register to track your shared secrets.
        </p>
      </div>

      <form onSubmit={handleSubmit} className="space-y-4">
        <div>
          <label
            htmlFor="display-name"
            className="block text-sm font-medium text-gray-700 dark:text-gray-300"
          >
            Display Name
          </label>
          <input
            id="display-name"
            type="text"
            value={displayName}
            onChange={(e) => setDisplayName(e.target.value)}
            onBlur={() => validateField("displayName", displayName)}
            className="mt-1 block w-full rounded-md border border-gray-300 dark:border-gray-600 dark:bg-gray-800 dark:text-white px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          />
          {errors.displayName && (
            <p className="mt-1 text-sm text-red-600 dark:text-red-400">{errors.displayName}</p>
          )}
        </div>

        <div>
          <label
            htmlFor="email"
            className="block text-sm font-medium text-gray-700 dark:text-gray-300"
          >
            Email
          </label>
          <input
            id="email"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            onBlur={() => validateField("email", email)}
            className="mt-1 block w-full rounded-md border border-gray-300 dark:border-gray-600 dark:bg-gray-800 dark:text-white px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          />
          {errors.email && (
            <p className="mt-1 text-sm text-red-600 dark:text-red-400">{errors.email}</p>
          )}
        </div>

        <div>
          <label
            htmlFor="password"
            className="block text-sm font-medium text-gray-700 dark:text-gray-300"
          >
            Password
          </label>
          <input
            id="password"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            onBlur={() => validateField("password", password)}
            className="mt-1 block w-full rounded-md border border-gray-300 dark:border-gray-600 dark:bg-gray-800 dark:text-white px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          />
          {errors.password && (
            <p className="mt-1 text-sm text-red-600 dark:text-red-400">{errors.password}</p>
          )}
          <p className="mt-1 text-xs text-gray-400">Minimum 8 characters</p>
        </div>

        <button
          type="submit"
          disabled={loading}
          className="flex w-full items-center justify-center gap-2 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
        >
          {loading && <Spinner size="sm" />}
          {loading ? "Creating account..." : "Create Account"}
        </button>
      </form>

      <p className="text-center text-sm text-gray-500 dark:text-gray-400">
        Already have an account?{" "}
        <Link
          to="/login"
          className="text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300"
        >
          Sign in
        </Link>
      </p>
    </div>
  );
}
