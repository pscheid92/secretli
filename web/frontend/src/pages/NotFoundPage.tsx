import { Link } from "react-router";

export default function NotFoundPage() {
  return (
    <div className="text-center py-16">
      <h1 className="text-4xl font-bold mb-4 dark:text-white">404</h1>
      <p className="text-gray-500 dark:text-gray-400 mb-6">Page not found.</p>
      <Link to="/" className="text-blue-600 hover:underline dark:text-blue-400">
        Go back home
      </Link>
    </div>
  );
}
