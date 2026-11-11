// The same function with an off-by-one: an order of exactly 100 still pays shipping.
export default function shippingFee({ subtotal, member }) {
  if (member || subtotal > 100) return { fee: 0 };
  return { fee: 15 };
}
