namespace Lastro.Tests;

/// <summary>The decision core of apps/docs/examples/shipping (shipping.mjs and shipping-buggy.mjs), as .NET subjects.</summary>
public static class ShippingExample
{
    public static Env Fee(Env input) => Decide(input, buggy: false);

    /// <summary>The off-by-one: an order of exactly 100 still pays shipping.</summary>
    public static Env BuggyFee(Env input) => Decide(input, buggy: true);

    private static Env Decide(Env input, bool buggy)
    {
        var subtotal = input["subtotal"].AsNumber;
        var free = buggy ? subtotal > 100 : subtotal >= 100;
        return Env.From(new Dictionary<string, object?> { ["fee"] = input["member"].AsBool || free ? 0 : 15 });
    }
}
