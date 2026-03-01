import { useState } from "react";
import { Controller, useForm } from "react-hook-form";
import ExpirationPicker from "./ExpirationPicker";
import Spinner from "./Spinner";
import Toggle from "./Toggle";

export interface SecretFormData {
  text: string;
  expiration: string;
  burnAfterRead: boolean;
  password: string;
}

interface SecretFormProps {
  onSubmit: (data: SecretFormData) => void;
  loading: boolean;
}

export default function SecretForm({ onSubmit, loading }: SecretFormProps) {
  const [showPassword, setShowPassword] = useState(false);

  const {
    register,
    handleSubmit,
    control,
    setValue,
    watch,
    formState: { errors },
  } = useForm<SecretFormData>({
    defaultValues: {
      text: "",
      expiration: "1d",
      burnAfterRead: false,
      password: "",
    },
  });

  const burnAfterRead = watch("burnAfterRead");
  const text = watch("text");

  return (
    <form onSubmit={handleSubmit(onSubmit)} className="space-y-5">
      {/* Textarea */}
      <div>
        <textarea
          id="secret-text"
          {...register("text", { required: "Secret text is required" })}
          placeholder="Type or paste your secret here..."
          rows={7}
          className="block w-full h-[168px] rounded-lg border border-zinc-200 dark:border-zinc-500 bg-white dark:bg-zinc-800 text-zinc-900 dark:text-zinc-100 px-4 py-3 text-sm placeholder:text-zinc-500 dark:placeholder:text-zinc-500 focus:outline-none focus:border-amber-400 dark:focus:border-amber-400 focus:ring-1 focus:ring-amber-400/20 transition-colors duration-150 resize-none"
        />
        {errors.text && (
          <p className="mt-1.5 text-xs text-red-500 dark:text-red-400">{errors.text.message}</p>
        )}
      </div>

      {/* Expiration */}
      <div className="space-y-2">
        <label className="block text-xs tracking-widest uppercase text-zinc-600 dark:text-zinc-100">
          Expires in
        </label>
        <Controller
          name="expiration"
          control={control}
          render={({ field }) => (
            <ExpirationPicker value={field.value} onChange={field.onChange} />
          )}
        />
      </div>

      {/* Options */}
      <div className="rounded-lg border border-zinc-200 dark:border-zinc-500 bg-white dark:bg-zinc-900 divide-y divide-zinc-200 dark:divide-zinc-500/60">
        <div className="px-4 py-3">
          <Toggle
            checked={burnAfterRead}
            onChange={() => setValue("burnAfterRead", !burnAfterRead, { shouldValidate: true })}
            label="Burn after reading"
            description="Permanently destroyed after first view"
          />
        </div>
        <div className="px-4 py-3">
          <Toggle
            checked={showPassword}
            onChange={() => {
              setShowPassword(!showPassword);
              if (showPassword) setValue("password", "");
            }}
            label="Password protection"
            description="Require a password to decrypt"
          />
        </div>
        {showPassword && (
          <div className="px-4 py-3">
            <input
              type="password"
              {...register("password", {
                validate: (v) => !showPassword || v.length > 0 || "Password is required",
              })}
              placeholder="Enter a password..."
              autoFocus
              className="w-full rounded-md border border-zinc-200 dark:border-zinc-500 bg-zinc-50 dark:bg-zinc-800 text-zinc-900 dark:text-zinc-100 px-3 py-2.5 text-sm placeholder:text-zinc-500 dark:placeholder:text-zinc-500 focus:outline-none focus:border-amber-400 dark:focus:border-amber-400 focus:ring-1 focus:ring-amber-400/20 transition-colors duration-150"
            />
            {errors.password && (
              <p className="mt-1.5 text-xs text-red-500 dark:text-red-400">{errors.password.message}</p>
            )}
          </div>
        )}
      </div>

      {/* Submit */}
      <button
        type="submit"
        disabled={loading || !text.trim()}
        className="flex w-full items-center justify-center gap-2 rounded-lg bg-amber-400 px-4 py-3 text-sm font-medium text-zinc-900 hover:bg-amber-300 focus:outline-none focus:ring-2 focus:ring-amber-400/50 disabled:opacity-40 disabled:cursor-not-allowed transition-all duration-150"
      >
        {loading && <Spinner size="sm" className="text-zinc-700" />}
        {loading ? "Encrypting..." : "Encrypt & Share"}
      </button>
    </form>
  );
}
