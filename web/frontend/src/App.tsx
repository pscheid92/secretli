import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter, Route, Routes } from "react-router";
import { Toaster } from "sonner";
import Layout from "./components/Layout";
import FilePage from "./pages/FilePage";
import NotFoundPage from "./pages/NotFoundPage";
import RetrievePage from "./pages/RetrievePage";
import SharePage from "./pages/SharePage";

const queryClient = new QueryClient();

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <Toaster richColors position="bottom-right" closeButton />
      <BrowserRouter>
        <Routes>
          <Route element={<Layout />}>
            <Route index element={<SharePage />} />
            <Route path="share" element={<SharePage />} />
            <Route path="s" element={<RetrievePage />} />
            <Route path="file" element={<FilePage />} />
            <Route path="*" element={<NotFoundPage />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </QueryClientProvider>
  );
}
