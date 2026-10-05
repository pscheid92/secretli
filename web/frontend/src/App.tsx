import { BrowserRouter, Route, Routes } from "react-router";
import Layout from "./components/Layout";
import FilePage from "./pages/FilePage";
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
          {/* Keyed: switching between /s and /c starts the page afresh. */}
          <Route path="s" element={<RetrievePage key="s" />} />
          <Route path="c" element={<RetrievePage key="c" />} />
          <Route path="file" element={<FilePage />} />
          <Route path="*" element={<NotFoundPage />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}
