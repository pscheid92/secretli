import { Link } from "react-router";

export default function NotFoundPage() {
  return (
    <div className="text-center py-24">
      <h1 className="text-6xl font-extrabold text-gray-300 dark:text-gray-700 mb-4">404</h1>
      <p className="text-gray-500 dark:text-gray-400 mb-6">Page not found.</p>
      <Link
        to="/"
        className="font-medium text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300 transition-colors duration-150"
      >
        Go back home
      </Link>
    </div>
  );
}
