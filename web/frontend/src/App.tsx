import { BrowserRouter, Route, Routes } from "react-router";
import { Toaster } from "sonner";
import Layout from "./components/Layout";
import FilePage from "./pages/FilePage";
import NotFoundPage from "./pages/NotFoundPage";
import RetrievePage from "./pages/RetrievePage";
import SharePage from "./pages/SharePage";

export default function App() {
  return (
    <>
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
    </>
  );
}
