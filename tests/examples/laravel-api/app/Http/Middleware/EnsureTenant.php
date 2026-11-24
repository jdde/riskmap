<?php

namespace App\Http\Middleware;

use Closure;
use Illuminate\Http\Request;

class EnsureTenant
{
    public function handle(Request $request, Closure $next)
    {
        abort_if($request->user()?->tenant_id === null, 403);

        return $next($request);
    }
}
