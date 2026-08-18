import { lazy, Suspense } from "react"
import { BrowserRouter, Route, Routes } from "react-router-dom"
import { AuthProvider } from "@/lib/auth"
import { AppShell } from "@/components/shell"
import { RepoLayout } from "@/components/repo-layout"
import { PageLoading } from "@/components/shared"

const Landing = lazy(() => import("@/pages/landing"))
const Login = lazy(() => import("@/pages/login"))
const Register = lazy(() => import("@/pages/register"))
const Dashboard = lazy(() => import("@/pages/dashboard"))
const Explore = lazy(() => import("@/pages/explore"))
const Profile = lazy(() => import("@/pages/profile"))
const NewRepo = lazy(() => import("@/pages/new-repo"))
const NewOrg = lazy(() => import("@/pages/new-org"))
const Notifications = lazy(() => import("@/pages/notifications"))
const Search = lazy(() => import("@/pages/search"))
const ImportRepo = lazy(() => import("@/pages/import"))
const UserSettings = lazy(() => import("@/pages/user-settings"))
const NotFound = lazy(() => import("@/pages/not-found"))

const Code = lazy(() => import("@/pages/repo/code"))
const Commits = lazy(() => import("@/pages/repo/commits"))
const Commit = lazy(() => import("@/pages/repo/commit"))
const Branches = lazy(() => import("@/pages/repo/branches"))
const Compare = lazy(() => import("@/pages/repo/compare"))
const Pulls = lazy(() => import("@/pages/repo/pulls"))
const PullNew = lazy(() => import("@/pages/repo/pull-new"))
const Pull = lazy(() => import("@/pages/repo/pull"))
const Stacks = lazy(() => import("@/pages/repo/stacks"))
const Issues = lazy(() => import("@/pages/repo/issues"))
const IssueNew = lazy(() => import("@/pages/repo/issue-new"))
const Issue = lazy(() => import("@/pages/repo/issue"))
const CI = lazy(() => import("@/pages/repo/ci"))
const CIRun = lazy(() => import("@/pages/repo/ci-run"))
const RepoSettings = lazy(() => import("@/pages/repo/settings"))
const RepoDeployments = lazy(() => import("@/pages/repo/deployments"))

export default function App() {
  return (
    <AuthProvider>
      <BrowserRouter>
        <Suspense fallback={<PageLoading />}>
          <Routes>
            {/* standalone marketing + auth pages (no app chrome) */}
            <Route path="/" element={<Landing />} />
            <Route path="/login" element={<Login />} />
            <Route path="/register" element={<Register />} />

            {/* product pages inside the app shell */}
            <Route element={<AppShell />}>
              <Route path="/dashboard" element={<Dashboard />} />
              <Route path="/explore" element={<Explore />} />
              <Route path="/new" element={<NewRepo />} />
              <Route path="/organizations/new" element={<NewOrg />} />
              <Route path="/notifications" element={<Notifications />} />
              <Route path="/search" element={<Search />} />
              <Route path="/import" element={<ImportRepo />} />
              <Route path="/settings" element={<UserSettings />} />
              <Route path="/:owner" element={<Profile />} />
              <Route path="/:owner/:repo" element={<RepoLayout />}>
                <Route index element={<Code />} />
                <Route path="tree/*" element={<Code />} />
                <Route path="blob/*" element={<Code />} />
                <Route path="commits/*" element={<Commits />} />
                <Route path="commits" element={<Commits />} />
                <Route path="commit/:sha" element={<Commit />} />
                <Route path="branches" element={<Branches />} />
                <Route path="compare" element={<Compare />} />
                <Route path="pulls" element={<Pulls />} />
                <Route path="pulls/new" element={<PullNew />} />
                <Route path="pull/:number" element={<Pull />} />
                <Route path="pull/:number/:tab" element={<Pull />} />
                <Route path="stacks" element={<Stacks />} />
                <Route path="issues" element={<Issues />} />
                <Route path="issues/new" element={<IssueNew />} />
                <Route path="issue/:number" element={<Issue />} />
                <Route path="ci" element={<CI />} />
                <Route path="ci/:number" element={<CIRun />} />
                <Route path="deployments" element={<RepoDeployments />} />
                <Route path="settings" element={<RepoSettings />} />
              </Route>
              <Route path="*" element={<NotFound />} />
            </Route>
          </Routes>
        </Suspense>
      </BrowserRouter>
    </AuthProvider>
  )
}
