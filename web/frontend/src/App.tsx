import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter, Route, Routes } from "react-router";
import { Toaster } from "sonner";
import Layout from "./components/Layout";
import { AuthProvider } from "./context/AuthContext";
import FilePage from "./pages/FilePage";
import HistoryPage from "./pages/HistoryPage";
import LoginPage from "./pages/LoginPage";
import NotFoundPage from "./pages/NotFoundPage";
import RegisterPage from "./pages/RegisterPage";
import RetrievePage from "./pages/RetrievePage";
import SharePage from "./pages/SharePage";

const queryClient = new QueryClient();

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <AuthProvider>
        <Toaster richColors position="top-right" />
        <BrowserRouter>
          <Routes>
            <Route element={<Layout />}>
              <Route index element={<SharePage />} />
              <Route path="s" element={<RetrievePage />} />
              <Route path="file" element={<FilePage />} />
              <Route path="login" element={<LoginPage />} />
              <Route path="register" element={<RegisterPage />} />
              <Route path="history" element={<HistoryPage />} />
              <Route path="*" element={<NotFoundPage />} />
            </Route>
          </Routes>
        </BrowserRouter>
      </AuthProvider>
    </QueryClientProvider>
  );
}
