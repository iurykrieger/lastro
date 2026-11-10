// The decision core under proof: a pure function from the model's inputs to its outputs.
export default function shippingFee({ subtotal, member }) {
  if (member || subtotal >= 100) return { fee: 0 };
  return { fee: 15 };
}
