import { Link } from "react-router-dom"
import { ArrowLeft } from "lucide-react"
import { Button } from "@/components/ui/button"
import { LogoMark } from "@/components/logo"

export default function NotFound() {
  return (
    <div className="relative flex flex-col items-center justify-center gap-5 overflow-hidden py-32 text-center">
      <LogoMark
        size={340}
        className="pointer-events-none absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 opacity-[0.05]"
      />
      <div className="relative">
        <div className="text-8xl font-extrabold tracking-tight sm:text-9xl">404</div>
        <p className="mt-3 text-muted-foreground">This page could not be found.</p>
      </div>
      <Button asChild className="relative">
        <Link to="/">
          <ArrowLeft className="size-4" />
          Take me home
        </Link>
      </Button>
    </div>
  )
}
