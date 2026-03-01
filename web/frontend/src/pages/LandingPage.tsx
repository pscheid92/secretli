import { Link } from "react-router";

export default function LandingPage() {
  return (
    <div className="flex flex-col items-center justify-center py-28 text-center">
      <h1 className="text-6xl font-extrabold tracking-tight bg-gradient-to-r from-blue-600 to-violet-600 bg-clip-text text-transparent">
        Secretli
      </h1>
      <p className="mt-4 text-xl max-w-md text-gray-500 dark:text-gray-400">
        Share your secrets secretly
      </p>
      <div className="mt-10 flex gap-4">
        <Link
          to="/share"
          className="rounded-lg bg-blue-600 px-8 py-3.5 text-base font-semibold text-white shadow-md hover:bg-blue-700 hover:shadow-lg focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 transition-all duration-150"
        >
          Share a Secret
        </Link>
        <Link
          to="/s"
          className="rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 px-8 py-3.5 text-base font-semibold text-gray-700 dark:text-gray-300 shadow-md hover:bg-gray-50 dark:hover:bg-gray-700 hover:shadow-lg focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 transition-all duration-150"
        >
          Retrieve a Secret
        </Link>
      </div>
    </div>
  );
}
