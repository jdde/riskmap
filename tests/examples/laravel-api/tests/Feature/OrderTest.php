<?php

namespace Tests\Feature;

use App\Models\User;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Tests\TestCase;

class OrderTest extends TestCase
{
    use RefreshDatabase;

    public function test_a_user_can_create_an_order(): void
    {
        $user = User::factory()->create(['tenant_id' => 1]);

        $this->actingAs($user)
            ->postJson('/api/orders', ['total_cents' => 1000])
            ->assertCreated();
    }
}
