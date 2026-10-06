import { BrowserRouter, Navigate, Route, Routes } from "react-router";
import Layout from "./components/Layout";
import HowItWorksPage from "./pages/HowItWorksPage";
import NotFoundPage from "./pages/NotFoundPage";
import RetrievePage from "./pages/RetrievePage";
import SharePage from "./pages/SharePage";

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route element={<Layout />}>
          <Route index element={<SharePage />} />
          <Route path="share" element={<SharePage />} />
          {/* Files are shared from the same page now; old links still land. */}
          <Route path="file" element={<Navigate to="/share" replace />} />
          {/* Keyed: switching between /s and /c starts the page afresh. */}
          <Route path="s" element={<RetrievePage key="s" />} />
          <Route path="c" element={<RetrievePage key="c" />} />
          <Route path="how" element={<HowItWorksPage />} />
          <Route path="*" element={<NotFoundPage />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}
